# Campaign lifecycle: a code-reading guide

This guide follows a campaign from the customer form to delivery and reports. It is a map for reading the code, not a specification of every validation rule. Function names are included so you can search for them if code moves. Paths below are relative to the backend root unless they start with `../jazebeh.ir-frontend`.

## 1. The layers

```text
Frontend form → HTTP route → handler → business flow → repository → database
                                  ↑          ↓
                             request/response DTOs    models
                                                        ↓
                          scheduler / bot flow → delivery provider
```

- **Routes** register URLs and middleware: [`app/router/routes.go`](../../app/router/routes.go). Campaign routes require authentication; admin and bot routes have their own access controls. Read the middleware and route group before assuming who can call an endpoint.
- **Handlers** translate HTTP requests into flow calls and flow results/errors into responses. Customer, admin, and bot handlers are respectively [`campaign_handler.go`](../../app/handlers/campaign_handler.go), [`campaign_admin_handler.go`](../../app/handlers/campaign_admin_handler.go), and [`campaign_bot_handler.go`](../../app/handlers/campaign_bot_handler.go).
- **DTOs** define request and response shapes: [`app/dto/campaign_dto.go`](../../app/dto/campaign_dto.go). JSON encoding/decoding uses these structs; they are not database tables.
- **Business flows** hold most use-case rules and coordinate storage, wallet operations, and other services. Start with [`campaign_flow.go`](../../business_flow/campaign_flow.go), then [`campaign_admin_flow.go`](../../business_flow/campaign_admin_flow.go) and [`campaign_bot_flow.go`](../../business_flow/campaign_bot_flow.go).
- **Models** describe persisted entities, relationships and some domain rules. [`models/campaign.go`](../../models/campaign.go) maps `Campaign` to `campaigns`, with configuration in `Spec` (JSONB). It also defines status, phase, targeting methods, and methods such as `IsEditable` and `CanTransitionTo`.
- **Repositories** encapsulate persistence, generally using GORM. [`repository/campaign_repository.go`](../../repository/campaign_repository.go) embeds a base repository and adds campaign queries. Related repositories cover selections, reservations, report jobs and delivery records. Transactions and locks also matter when tracing writes.

Business logic is not exclusively in `business_flow`: model methods contain domain rules, handlers perform HTTP-level validation, and schedulers contain operational delivery logic. Check the actual caller as well as the model's transition helper: the helper alone is not a complete description of all operational transitions.

## 2. Lifecycle at a glance

```text
Create → initiated / editing → finalize → waiting-for-approval
                                             ├→ rejected / cancelled / expired
                                             └→ approved → running → executed
                                                               └→ interrupted
```

This is a simplified map, not an exhaustive state machine. `in-progress` and `cancelled-by-admin` are also defined. Read the relevant operation before assuming a transition is allowed. `interrupted` protects against unsafe automatic replay after a scheduler lease expires with durable delivery records. Phase (`test` or `execution`) is separate from status: phase describes the purpose of a campaign; status describes its progress. Platforms include SMS, Rubika, Bale and SPlus.

## 3. Creation and editing

1. The customer interface begins at [`CampaignCreationPage.tsx`](../../../jazebeh.ir-frontend/src/pages/CampaignCreationPage.tsx); [`useCampaign.tsx`](../../../jazebeh.ir-frontend/src/hooks/useCampaign.tsx) holds form state. API calls and TypeScript shapes are in [`services/api.ts`](../../../jazebeh.ir-frontend/src/services/api.ts) and [`types/campaign.ts`](../../../jazebeh.ir-frontend/src/types/campaign.ts). Follow the actual API call from a UI action rather than assuming every form step creates a separate endpoint.
2. `POST /api/v1/campaigns` reaches `CampaignHandler.CreateCampaign`. The handler reads a `CreateCampaignRequest`, obtains the authenticated customer context, and calls the flow. Compare the DTO fields with `CampaignSpec` and the response; they serve different purposes.
3. `CampaignFlowImpl.CreateCampaign` validates the request, loads the customer, normalizes input and checks references. Inside a transaction, `createCampaign` constructs and persists the campaign; smart targeting also stores selected tags. It records an audit event and returns ID, UUID, status and creation time. Follow `validateCreateCampaignRequest`, `ensureCreateCampaignRefs`, and `createCampaign` for details.
4. `PUT /api/v1/campaigns/:uuid` calls `UpdateCampaign`. It looks up the customer's campaign, checks whether it can be changed, validates effective values and references, and applies the update. The request can also ask to finalize. See `validateUpdateCampaignRequest`, `prepareAudienceTargetingUpdate`, and `applyCampaignSpecUpdate`.

A useful debugging path is: route → handler → DTO → flow method → helper → repository → model. Pay attention to ID versus UUID and to which values come from authenticated context instead of the request body.

## 4. Audience, cost and finalization

The campaign supports **standard**, **Excel**, and **smart targeting**. `CampaignSpec.EffectiveAudienceTargetingMethod` and the targeting helpers explain how the stored configuration is interpreted. A bundle, phase, tags, audience capacity and platform settings can affect which choices are valid; Excel follows a different audience path.

- `CalculateCampaignCapacity` and `CalculateCampaignCost` / `CalculateCampaignCostV2` in the customer flow calculate estimates. Price helpers account for inputs such as platform and price factors. The corresponding handler methods and DTOs show what each endpoint accepts and returns.
- For smart targeting, the customer handler exposes tag selection, capacity calculations, test sampling previews and execution-audience calculations. Follow its calls into the corresponding `smart_targeting` flows and repositories. Test and execution phases have different requirements; a preview or calculation is not itself delivery.
- In `UpdateCampaign`, a request with `Finalize` set triggers additional checks (`canFinalizeCampaign`), cost calculation and transactional work. The flow can reserve smart-targeting execution audiences, moves the campaign to `waiting-for-approval`, and records a wallet freeze transaction for the campaign budget. Inspect the transaction block for exact ordering and rollback behavior.
- A test message is a separate operation: see `SendCampaignTestMessage` in the handler and [`campaign_test_send.go`](../../business_flow/campaign_test_send.go). It should not be confused with the test campaign phase.

## 5. Admin review and alternatives

The admin handler exposes list/detail and approve, reject, reschedule and cancel operations. Follow `ApproveCampaign` into the admin flow: it locks and loads the campaign, requires `waiting-for-approval` and a valid future schedule, checks relevant audience/platform conditions, and finds the budget freeze transaction. Continue through the transaction to understand the wallet and status changes. Smart-targeting test campaigns additionally check their reserved sample snapshot.

Read `RejectCampaign`, `CancelCampaign` and `RescheduleCampaign` in the same flow for their distinct rules and financial effects. Customer `CancelCampaign` is in the customer flow; do not assume its permissions or transitions are identical to admin cancellation. The customer flow also contains `expireCustomerCampaigns` and refund reconciliation. [`campaign_refund_reconciliation_scheduler.go`](../../app/scheduler/campaign_refund_reconciliation_scheduler.go) runs reconciliation work; follow its flow and repository for eligibility and retry details.

## 6. Scheduled execution and delivery

Approved campaigns are made available to operational actors by `BotCampaignFlowImpl.ListReadyCampaigns` in [`campaign_bot_flow.go`](../../business_flow/campaign_bot_flow.go). Its status methods include `MoveCampaignToRunning` and `MoveCampaignToExecuted`. The bot handler exposes operational endpoints; read the route and authentication for the actor involved.

For a concrete delivery trace, read [`sms_campaign_scheduler.go`](../../app/scheduler/sms_campaign_scheduler.go):

1. `Start` and `runOnce` drive scheduled work; `processSMSCampaign` handles a campaign.
2. Validation, provider resolution and audience selection prepare messages. Follow the actual targeting branch for the campaign being investigated.
3. The scheduler sends batches and persists provider outcomes. Provider acceptance and final delivery are distinct events.
4. Status jobs are scheduled and later processed; processed campaign statistics are updated from delivery records.

Other platforms have their own schedulers: [`rubika_campaign_scheduler.go`](../../app/scheduler/rubika_campaign_scheduler.go), [`bale_campaign_scheduler.go`](../../app/scheduler/bale_campaign_scheduler.go), and [`splus_campaign_scheduler.go`](../../app/scheduler/splus_campaign_scheduler.go). Their behavior is not necessarily identical to SMS. For concurrency, recovery and audience reservation details, follow the scheduler's helpers and [`campaign_execution_repository.go`](../../repository/campaign_execution_repository.go), rather than assuming a status change alone means every recipient was delivered.

## 7. Results, reports and follow-up

Campaign statistics and audience information are used by list/detail and export operations in the customer flow. Start with `ListCampaigns`, `ExportCampaignReport`, and `ExportCampaignAudienceClickReport`; look at `Campaign.Statistics`, `processed_campaign` and delivery repositories for persisted data. The frontend report entry point is [`ReportsPage.tsx`](../../../jazebeh.ir-frontend/src/pages/ReportsPage.tsx).

Large audience click reports have a separate job lifecycle: the handler creates a job, returns its state, and provides a download endpoint. Follow [`campaign_audience_report_job_flow.go`](../../business_flow/campaign_audience_report_job_flow.go), its repository/model, and [`campaign_audience_report_scheduler.go`](../../app/scheduler/campaign_audience_report_scheduler.go) for execution and cleanup. A queued job is not yet a completed report.

Other customer operations include cloning and hiding/unhiding campaigns. They are exposed by the customer handler and implemented in the campaign flow; hiding affects visibility, whereas cancellation changes the campaign lifecycle.

## Suggested reading exercise

Trace a single create request using `CreateCampaignRequest` → handler → `CampaignFlowImpl.CreateCampaign` → `createCampaign` → repository → `Campaign`. Then trace the same campaign through `UpdateCampaign` with `Finalize`, admin `ApproveCampaign`, and the SMS scheduler. At each step write down: the actor, input, current status required, repositories touched, transaction boundary, resulting status, and possible errors. Finally compare with a smart-targeting or Excel branch and read the nearby tests for concrete examples and edge cases.
## Transaction and recovery reading notes

When following finalization into execution, distinguish a proposed audience, its temporary reservation, and the permanent bundle selection. Reservation readers validate the frozen members before materialization; cancellation or invalidation releases claims under locks. See [execution reservations](campaign-targeting-execution-reservation-repository.md), [test sample selections](campaign-targeting-test-sample-selection-repository.md), and [bundle audience selections](bundle-audience-selection-repository.md) for their method contracts.

For a failed or stale scheduler run, follow the persisted send intent rather than assuming no provider request occurred. The [campaign execution guide](campaign-execution-repository.md) explains why recovery requeues only runs without delivery intent and interrupts the others. Provider status jobs and refund reconciliation have separate lifecycles; see the [status job](campaign-status-job-repository.md) and [refund reconciliation](campaign-refund-reconciliation-repository.md) guides.
