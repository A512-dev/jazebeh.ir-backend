package repository

import (
	"context"
	"fmt"
	"strings"

	"github.com/amirphl/Yamata-no-Orochi/models"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

// SmartTargetingSelectionPhase is explicit rather than inferred from whether
// one particular exclusion happens to be enabled. This keeps Test and
// execution capacity semantics independently auditable as new query modes are
// introduced.
type SmartTargetingSelectionPhase string

const (
	SmartTargetingSelectionPhaseTest      SmartTargetingSelectionPhase = "test"
	SmartTargetingSelectionPhaseExecution SmartTargetingSelectionPhase = "execution"
)

// SmartTargetingAudienceQuery is the immutable eligibility contract supplied
// to one capacity, Test-preview, or Execution-selection operation. Every
// query in this repository applies the same Bundle-consumption exclusions;
// callers should build it from the campaign's current or persisted snapshot,
// never from partially updated request fields.
type SmartTargetingAudienceQuery struct {
	// BundleID scopes consumed-audience, reservation, and exclusion checks.
	BundleID uint
	// Phase keeps Test and Execution behavior explicit even when their current
	// eligibility SQL happens to look similar.
	Phase SmartTargetingSelectionPhase
	// ExcludeActiveTestReservationCampaignID keeps a campaign's own active
	// Test reservation in its capacity population. A finalized Test campaign
	// can refresh capacity before execution; its already-reserved sample is not
	// prior use by another campaign and must not make its own capacity collapse.
	// Zero retains the normal behavior of excluding every active reservation.
	ExcludeActiveTestReservationCampaignID uint
	// ExcludeActiveExecutionReservationCampaignID has the same purpose for an
	// execution campaign that has already frozen its concrete reservation.
	ExcludeActiveExecutionReservationCampaignID uint
	// TagIDs is the selected-tag union used for capacity and percentile bounds.
	TagIDs []int64
	// ScoreClasses is a canonical non-empty subset of A, B, and C. Selecting
	// all three classes skips percentile work and includes unscored profiles.
	ScoreClasses []string
	// AllowedColors is empty when the platform/provider has no colour
	// restriction; otherwise profiles must have one of these colours.
	AllowedColors []string
}

type SmartTargetingAudienceRepository interface {
	// CalculateCapacity counts the selected-tag union before and after eligibility
	// consumption rules and requested score-class filtering.
	CalculateCapacity(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingAudienceCapacity, error)
	// SelectCandidates returns deterministic score-ranked Execution candidates.
	SelectCandidates(ctx context.Context, query SmartTargetingAudienceQuery, limit int64) ([]*models.AudienceProfile, error)
	// CalculateScoreBounds returns one selected-tag-union p33/p66 pair for
	// reuse by every tag query in one Test sampling operation.
	CalculateScoreBounds(ctx context.Context, query SmartTargetingAudienceQuery) (*SmartTargetingScoreBounds, error)
	// SelectIDsForTag returns only IDs for a single selected Test tag.
	SelectIDsForTag(
		ctx context.Context,
		query SmartTargetingAudienceQuery,
		bounds *SmartTargetingScoreBounds,
		tagID int64,
		excludeAudienceIDs []int64,
		limit int64,
	) ([]int64, error)
	// SelectForTag returns the complete profile fields needed to persist one
	// single-tag Test-preview member.
	SelectForTag(
		ctx context.Context,
		query SmartTargetingAudienceQuery,
		bounds *SmartTargetingScoreBounds,
		tagID int64,
		excludeAudienceIDs []int64,
		limit int64,
	) ([]*models.AudienceProfile, error)
}

// SmartTargetingAudienceCapacity separates the tag-union size before Bundle
// consumption exclusions (RawUniqueCount) from the usable size after those
// exclusions and any requested score-class filter (EligibleUniqueCount).
type SmartTargetingAudienceCapacity struct {
	RawUniqueCount      int64 `gorm:"column:raw_unique_count"`
	EligibleUniqueCount int64 `gorm:"column:eligible_unique_count"`
}

// SmartTargetingScoreBounds contains the union-wide score boundaries used by
// every per-tag selection in one operation. Nil boundaries mean the eligible
// union has no scored profiles, so every restricted score class is empty.
type SmartTargetingScoreBounds struct {
	P33 *float64 `gorm:"column:p33"`
	P66 *float64 `gorm:"column:p66"`
}

type smartTargetingAudienceRepository struct{ db *gorm.DB }

func NewSmartTargetingAudienceRepository(db *gorm.DB) SmartTargetingAudienceRepository {
	return &smartTargetingAudienceRepository{db: db}
}

func (r *smartTargetingAudienceRepository) getDB(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(TxContextKey).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return r.db.WithContext(ctx)
}

// smartTargetingEligiblePopulationBaseCTE defines the smallest shared
// population: profiles matching the selected-tag union, reachable by delivery,
// and not already consumed, reserved, committed, or explicitly excluded in
// this Bundle. Keep its bind order synchronized with
// smartTargetingEligiblePopulationArgs.
//
// It intentionally selects only ID and score; capacity and percentile queries
// do not need delivery payload columns.
const smartTargetingEligiblePopulationBaseCTE = `
WITH tagged_population AS (
    SELECT ap.id, ap.normalized_score
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
      AND (?::boolean OR ap.color = ANY(?::text[]))
), candidate_population AS (
    SELECT tagged.*
    FROM tagged_population AS tagged
    WHERE NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
      WHERE used.bundle_id = ? AND used.audience_id = tagged.id
      )
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_test_sample_reservations AS reserved
          WHERE reserved.bundle_id = ? AND reserved.audience_id = tagged.id
            AND reserved.state = 'active'
			AND (?::bigint = 0 OR reserved.campaign_id <> ?)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_execution_reservations AS execution_reserved
          WHERE execution_reserved.bundle_id = ? AND execution_reserved.audience_id = tagged.id
            AND execution_reserved.state = 'active'
			AND (?::bigint = 0 OR execution_reserved.campaign_id <> ?)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_execution_calculation_members AS execution_member
          JOIN campaign_targeting_execution_calculations AS execution_calculation
            ON execution_calculation.id = execution_member.calculation_id
          WHERE execution_calculation.bundle_id = ?
            AND execution_member.audience_id = tagged.id
            AND execution_calculation.status = 'committed'
			AND (?::bigint = 0 OR execution_calculation.campaign_id <> ?)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_exclusions AS bundle_exclusion
          WHERE bundle_exclusion.bundle_id = ?
            AND bundle_exclusion.audience_id = tagged.id
      )
)`

// smartTargetingSelectableAudienceBaseCTE has the same eligibility semantics
// as smartTargetingEligiblePopulationBaseCTE, but retains fields required to
// persist and later deliver exact candidate selections. Capacity and
// percentile work use the slim CTE above to avoid materializing them.
const smartTargetingSelectableAudienceBaseCTE = `
WITH tagged_population AS (
    SELECT ap.id, ap.uid, ap.phone_number, ap.tags, ap.normalized_score
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
      AND (?::boolean OR ap.color = ANY(?::text[]))
), candidate_population AS (
    SELECT tagged.*
    FROM tagged_population AS tagged
    WHERE NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
      WHERE used.bundle_id = ? AND used.audience_id = tagged.id
      )
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_test_sample_reservations AS reserved
          WHERE reserved.bundle_id = ? AND reserved.audience_id = tagged.id
            AND reserved.state = 'active'
			AND (?::bigint = 0 OR reserved.campaign_id <> ?)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_execution_reservations AS execution_reserved
          WHERE execution_reserved.bundle_id = ? AND execution_reserved.audience_id = tagged.id
            AND execution_reserved.state = 'active'
			AND (?::bigint = 0 OR execution_reserved.campaign_id <> ?)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_execution_calculation_members AS execution_member
          JOIN campaign_targeting_execution_calculations AS execution_calculation
            ON execution_calculation.id = execution_member.calculation_id
          WHERE execution_calculation.bundle_id = ?
            AND execution_member.audience_id = tagged.id
            AND execution_calculation.status = 'committed'
			AND (?::bigint = 0 OR execution_calculation.campaign_id <> ?)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_exclusions AS bundle_exclusion
          WHERE bundle_exclusion.bundle_id = ?
            AND bundle_exclusion.audience_id = tagged.id
      )
)`

// smartTargetingScoreClassificationCTE appends percentile bounds and a score
// class to either eligible-population CTE. Thresholds are based on every
// scored candidate in the selected-tag union: C is <= p33, B is (p33, p66],
// A is > p66, and nil scores remain unscored.
const smartTargetingScoreClassificationCTE = `, percentile_bounds AS (
    SELECT percentile_disc(ARRAY[0.33, 0.66]::double precision[])
               WITHIN GROUP (ORDER BY normalized_score) AS percentile_values
    FROM candidate_population
    WHERE normalized_score IS NOT NULL
), bounds AS (
    SELECT percentile_values[1] AS p33, percentile_values[2] AS p66
    FROM percentile_bounds
), classified AS (
    SELECT population.*,
        CASE
            WHEN population.normalized_score IS NULL THEN 'unscored'
            WHEN bounds.p33 IS NULL OR bounds.p66 IS NULL THEN 'unscored'
            WHEN population.normalized_score <= bounds.p33 THEN 'C'
            WHEN population.normalized_score <= bounds.p66 THEN 'B'
            ELSE 'A'
        END AS score_class
    FROM candidate_population AS population
    CROSS JOIN bounds
)`

const smartTargetingClassifiedEligiblePopulationCTE = smartTargetingEligiblePopulationBaseCTE + smartTargetingScoreClassificationCTE
const smartTargetingClassifiedSelectableAudienceCTE = smartTargetingSelectableAudienceBaseCTE + smartTargetingScoreClassificationCTE

// smartTargetingSelectsAllScoreClasses reports the one canonical form that
// means “do not filter by score.” It is intentionally strict so malformed or
// reordered caller input cannot silently bypass score-class filtering.
func smartTargetingSelectsAllScoreClasses(classes []string) bool {
	return len(classes) == 3 && classes[0] == "A" && classes[1] == "B" && classes[2] == "C"
}

// smartTargetingEligiblePopulationArgs returns bind variables for both shared
// eligible-population CTEs, in SQL placeholder order. An empty colour list is
// represented by a boolean bypass plus an empty typed array so the SQL shape
// remains stable.
func smartTargetingEligiblePopulationArgs(query SmartTargetingAudienceQuery) []any {
	colors := query.AllowedColors
	if colors == nil {
		colors = []string{}
	}
	return []any{
		pq.Array(query.TagIDs),
		len(colors) == 0,
		pq.Array(colors),
		query.BundleID,
		query.BundleID,
		query.ExcludeActiveTestReservationCampaignID,
		query.ExcludeActiveTestReservationCampaignID,
		query.BundleID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.BundleID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.BundleID,
	}
}

func (q SmartTargetingAudienceQuery) valid() bool {
	return q.BundleID != 0 && len(q.TagIDs) != 0 && len(q.ScoreClasses) != 0 &&
		(q.Phase == SmartTargetingSelectionPhaseTest || q.Phase == SmartTargetingSelectionPhaseExecution)
}

func (r *smartTargetingAudienceRepository) CalculateCapacity(
	ctx context.Context,
	query SmartTargetingAudienceQuery,
) (*SmartTargetingAudienceCapacity, error) {
	if !query.valid() {
		return nil, fmt.Errorf("invalid smart-targeting audience count query")
	}
	var capacity SmartTargetingAudienceCapacity
	if smartTargetingSelectsAllScoreClasses(query.ScoreClasses) {
		sql := smartTargetingEligiblePopulationBaseCTE + `
SELECT (SELECT COUNT(*) FROM tagged_population) AS raw_unique_count,
       (SELECT COUNT(*) FROM candidate_population) AS eligible_unique_count`
		err := r.getDB(ctx).Raw(sql, smartTargetingEligiblePopulationArgs(query)...).Scan(&capacity).Error
		return &capacity, err
	}
	sql := smartTargetingClassifiedEligiblePopulationCTE + `
SELECT (SELECT COUNT(*) FROM tagged_population) AS raw_unique_count,
       COUNT(*) FILTER (WHERE (?::boolean OR score_class = ANY(?::text[]))) AS eligible_unique_count
FROM classified`
	args := append(smartTargetingEligiblePopulationArgs(query),
		smartTargetingSelectsAllScoreClasses(query.ScoreClasses), pq.Array(query.ScoreClasses))
	err := r.getDB(ctx).Raw(sql, args...).Scan(&capacity).Error
	return &capacity, err
}

func (r *smartTargetingAudienceRepository) SelectCandidates(
	ctx context.Context,
	query SmartTargetingAudienceQuery,
	limit int64,
) ([]*models.AudienceProfile, error) {
	if !query.valid() || limit <= 0 {
		return nil, fmt.Errorf("invalid smart-targeting audience selection query")
	}
	var rows []*models.AudienceProfile
	if smartTargetingSelectsAllScoreClasses(query.ScoreClasses) {
		sql := smartTargetingSelectableAudienceBaseCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM candidate_population
ORDER BY normalized_score DESC NULLS LAST, id ASC
LIMIT ?`
		args := append(smartTargetingEligiblePopulationArgs(query), limit)
		err := r.getDB(ctx).Raw(sql, args...).Scan(&rows).Error
		return rows, err
	}
	sql := smartTargetingClassifiedSelectableAudienceCTE + `
SELECT id, uid, phone_number, tags, normalized_score
FROM classified
WHERE (?::boolean OR score_class = ANY(?::text[]))
ORDER BY normalized_score DESC NULLS LAST, id ASC
LIMIT ?`
	args := append(smartTargetingEligiblePopulationArgs(query),
		smartTargetingSelectsAllScoreClasses(query.ScoreClasses), pq.Array(query.ScoreClasses), limit)
	err := r.getDB(ctx).Raw(sql, args...).Scan(&rows).Error
	return rows, err
}

// CalculateScoreBounds computes the selected-tag union's exact p33/p66 once
// per sampling operation. Per-tag sampling then applies these same thresholds
// to every selected tag, rather than rebuilding and sorting the union for each
// tag. All score classes need no bounds because they include every candidate.
func (r *smartTargetingAudienceRepository) CalculateScoreBounds(
	ctx context.Context,
	query SmartTargetingAudienceQuery,
) (*SmartTargetingScoreBounds, error) {
	if !query.valid() {
		return nil, fmt.Errorf("invalid smart-targeting score-bound query")
	}
	if smartTargetingSelectsAllScoreClasses(query.ScoreClasses) {
		return &SmartTargetingScoreBounds{}, nil
	}
	sql, args := smartTargetingScoreBoundsQuery(query)
	var bounds SmartTargetingScoreBounds
	if err := r.getDB(ctx).Raw(sql, args...).Scan(&bounds).Error; err != nil {
		return nil, err
	}
	return &bounds, nil
}

// SelectIDsForTag remains available for count-only callers. Test preview
// persistence uses SelectForTag so its immutable snapshot contains the exact
// selected profile IDs and tag attribution.
func (r *smartTargetingAudienceRepository) SelectIDsForTag(
	ctx context.Context,
	query SmartTargetingAudienceQuery,
	bounds *SmartTargetingScoreBounds,
	tagID int64,
	excludeAudienceIDs []int64,
	limit int64,
) ([]int64, error) {
	sql, args, err := buildSmartTargetingTestTagSelectionQuery(query, bounds, tagID, excludeAudienceIDs, limit, true)
	if err != nil {
		return nil, err
	}
	var ids []int64
	if err := r.getDB(ctx).Raw(sql, args...).Scan(&ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// SelectForTag is the Smart Targeting Test sampling path. Score classes
// restrict eligibility; no random, score, ID, or other ordering is imposed.
// TODO(smart-targeting-random-sampling): Without ORDER BY, PostgreSQL returns
// a planner/storage-dependent LIMIT result. It is therefore not statistically
// random and is not reproducible. ORDER BY random() is deliberately avoided
// because it is prohibitively expensive at the current population size; replace
// this with a performant indexed or seeded sampling strategy.
func (r *smartTargetingAudienceRepository) SelectForTag(
	ctx context.Context,
	query SmartTargetingAudienceQuery,
	bounds *SmartTargetingScoreBounds,
	tagID int64,
	excludeAudienceIDs []int64,
	limit int64,
) ([]*models.AudienceProfile, error) {
	sql, args, err := buildSmartTargetingTestTagSelectionQuery(query, bounds, tagID, excludeAudienceIDs, limit, false)
	if err != nil {
		return nil, err
	}
	var rows []*models.AudienceProfile
	if err := r.getDB(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// smartTargetingScoreBoundsQuery returns only the p33/p66 pair, not candidate
// rows or a classified dataset. It intentionally duplicates the base-population
// eligibility predicates so PostgreSQL can aggregate the narrow profile set
// directly rather than materializing the classification CTE. Keep these
// predicates equivalent to the eligible-population CTE and Test tag selector.
func smartTargetingScoreBoundsQuery(query SmartTargetingAudienceQuery) (string, []any) {
	sql := `
	SELECT percentile_values[1] AS p33, percentile_values[2] AS p66
FROM (
    SELECT percentile_disc(ARRAY[0.33, 0.66]::double precision[])
               WITHIN GROUP (ORDER BY ap.normalized_score) AS percentile_values
    FROM audience_profiles AS ap
    WHERE ap.tags && ?::integer[]
      AND ap.phone_number IS NOT NULL
      AND BTRIM(ap.phone_number) <> ''
      AND ap.normalized_score IS NOT NULL`
	args := []any{pq.Array(query.TagIDs)}
	if len(query.AllowedColors) > 0 {
		sql += `
      AND ap.color = ANY(?::text[])`
		args = append(args, pq.Array(query.AllowedColors))
	}
	sql += `
	  AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_selection_members AS used
          WHERE used.bundle_id = ? AND used.audience_id = ap.id
      )`
	args = append(args, query.BundleID)
	sql += `
      AND NOT EXISTS (
          SELECT 1
      FROM campaign_targeting_test_sample_reservations AS reserved
      WHERE reserved.bundle_id = ? AND reserved.audience_id = ap.id
        AND reserved.state = 'active'
		AND (?::bigint = 0 OR reserved.campaign_id <> ?)
      )`
	args = append(args, query.BundleID, query.ExcludeActiveTestReservationCampaignID, query.ExcludeActiveTestReservationCampaignID)
	sql += `
	  AND NOT EXISTS (
      SELECT 1
      FROM campaign_targeting_execution_reservations AS execution_reserved
      WHERE execution_reserved.bundle_id = ? AND execution_reserved.audience_id = ap.id
        AND execution_reserved.state = 'active'
		AND (?::bigint = 0 OR execution_reserved.campaign_id <> ?)
  )`
	args = append(
		args,
		query.BundleID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.ExcludeActiveExecutionReservationCampaignID,
	)
	sql += `
      AND NOT EXISTS (
          SELECT 1
          FROM campaign_targeting_execution_calculation_members AS execution_member
          JOIN campaign_targeting_execution_calculations AS execution_calculation
            ON execution_calculation.id = execution_member.calculation_id
          WHERE execution_calculation.bundle_id = ?
            AND execution_member.audience_id = ap.id
            AND execution_calculation.status = 'committed'
			AND (?::bigint = 0 OR execution_calculation.campaign_id <> ?)
      )`
	args = append(
		args,
		query.BundleID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.ExcludeActiveExecutionReservationCampaignID,
	)
	sql += `
      AND NOT EXISTS (
          SELECT 1
          FROM bundle_audience_exclusions AS bundle_exclusion
          WHERE bundle_exclusion.bundle_id = ?
            AND bundle_exclusion.audience_id = ap.id
      )`
	args = append(args, query.BundleID)
	sql += `
) AS calculated_bounds`
	return sql, args
}

// buildSmartTargetingTestTagSelectionQuery returns one tag's currently
// eligible Test candidates. excludeAudienceIDs is the in-memory list already
// chosen for earlier tags in this same preview, preventing a profile from
// being selected twice before any reservation exists.
func buildSmartTargetingTestTagSelectionQuery(
	query SmartTargetingAudienceQuery,
	bounds *SmartTargetingScoreBounds,
	tagID int64,
	excludeAudienceIDs []int64,
	limit int64,
	selectIDsOnly bool,
) (string, []any, error) {
	if !query.valid() || tagID <= 0 || limit <= 0 {
		return "", nil, fmt.Errorf("invalid smart-targeting per-tag sample query")
	}
	isSelectedTag := false
	for _, candidateTagID := range query.TagIDs {
		if candidateTagID == tagID {
			isSelectedTag = true
			break
		}
	}
	if !isSelectedTag {
		return "", nil, fmt.Errorf("smart-targeting per-tag sample is not in the selected tag set")
	}
	if excludeAudienceIDs == nil {
		excludeAudienceIDs = []int64{}
	}
	selectColumns := "ap.id, ap.uid, ap.phone_number, ap.tags, ap.normalized_score"
	if selectIDsOnly {
		selectColumns = "ap.id"
	}
	sql := `
SELECT ` + selectColumns + `
FROM audience_profiles AS ap
WHERE ap.tags @> ARRAY[?]::integer[]
  AND ap.phone_number IS NOT NULL
  AND BTRIM(ap.phone_number) <> ''`
	args := []any{tagID}
	if len(query.AllowedColors) > 0 {
		sql += `
  AND ap.color = ANY(?::text[])`
		args = append(args, pq.Array(query.AllowedColors))
	}
	sql += `
	  AND NOT EXISTS (
      SELECT 1
      FROM bundle_audience_selection_members AS used
      WHERE used.bundle_id = ? AND used.audience_id = ap.id
  )`
	args = append(args, query.BundleID)
	sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM campaign_targeting_test_sample_reservations AS reserved
      WHERE reserved.bundle_id = ? AND reserved.audience_id = ap.id
        AND reserved.state = 'active'
		AND (?::bigint = 0 OR reserved.campaign_id <> ?)
  )`
	args = append(args, query.BundleID, query.ExcludeActiveTestReservationCampaignID, query.ExcludeActiveTestReservationCampaignID)
	sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM campaign_targeting_execution_reservations AS execution_reserved
      WHERE execution_reserved.bundle_id = ? AND execution_reserved.audience_id = ap.id
        AND execution_reserved.state = 'active'
		AND (?::bigint = 0 OR execution_reserved.campaign_id <> ?)
  )`
	args = append(
		args,
		query.BundleID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.ExcludeActiveExecutionReservationCampaignID,
	)
	sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM campaign_targeting_execution_calculation_members AS execution_member
      JOIN campaign_targeting_execution_calculations AS execution_calculation
        ON execution_calculation.id = execution_member.calculation_id
      WHERE execution_calculation.bundle_id = ?
        AND execution_member.audience_id = ap.id
        AND execution_calculation.status = 'committed'
		AND (?::bigint = 0 OR execution_calculation.campaign_id <> ?)
  )`
	args = append(
		args,
		query.BundleID,
		query.ExcludeActiveExecutionReservationCampaignID,
		query.ExcludeActiveExecutionReservationCampaignID,
	)
	sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM bundle_audience_exclusions AS bundle_exclusion
      WHERE bundle_exclusion.bundle_id = ?
        AND bundle_exclusion.audience_id = ap.id
  )`
	args = append(args, query.BundleID)
	sql += `
  AND NOT EXISTS (
      SELECT 1
      FROM unnest(?::bigint[]) AS earlier(audience_id)
      WHERE earlier.audience_id = ap.id
  )`
	args = append(args, pq.Array(excludeAudienceIDs))

	scorePredicate, scoreArgs, err := buildSmartTargetingScoreClassPredicate(query.ScoreClasses, bounds)
	if err != nil {
		return "", nil, err
	}
	sql += scorePredicate + `
LIMIT ?`
	args = append(args, scoreArgs...)
	args = append(args, limit)
	return sql, args, nil
}

// buildSmartTargetingScoreClassPredicate translates the selected score classes
// into a predicate over one tag's candidates. It uses precomputed union-wide
// bounds so tags cannot drift into different A/B/C definitions within a single
// Test-preview calculation.
func buildSmartTargetingScoreClassPredicate(classes []string, bounds *SmartTargetingScoreBounds) (string, []any, error) {
	key := strings.Join(classes, ",")
	if key == "A,B,C" {
		return "", nil, nil
	}
	if bounds == nil {
		return "", nil, fmt.Errorf("smart-targeting score bounds are required")
	}
	if (bounds.P33 == nil) != (bounds.P66 == nil) {
		return "", nil, fmt.Errorf("smart-targeting score bounds are inconsistent")
	}
	if bounds.P33 == nil {
		return "\n  AND FALSE", nil, nil
	}
	if *bounds.P33 > *bounds.P66 {
		return "", nil, fmt.Errorf("smart-targeting score bounds are invalid")
	}
	p33, p66 := *bounds.P33, *bounds.P66
	switch key {
	case "A":
		return "\n  AND ap.normalized_score > ?::double precision", []any{p66}, nil
	case "B":
		return "\n  AND ap.normalized_score > ?::double precision AND ap.normalized_score <= ?::double precision", []any{p33, p66}, nil
	case "C":
		return "\n  AND ap.normalized_score <= ?::double precision", []any{p33}, nil
	case "A,B":
		return "\n  AND ap.normalized_score > ?::double precision", []any{p33}, nil
	case "A,C":
		return "\n  AND (ap.normalized_score <= ?::double precision OR ap.normalized_score > ?::double precision)", []any{p33, p66}, nil
	case "B,C":
		return "\n  AND ap.normalized_score <= ?::double precision", []any{p66}, nil
	default:
		return "", nil, fmt.Errorf("invalid smart-targeting score classes")
	}
}
