# Backend Roadmap

## Tech Stack

| ส่วน | เทคโนโลยี |
|---|---|
| Language | Go |
| HTTP Router | chi |
| PostgreSQL Driver | pgx |
| SQL Code Generation | sqlc |
| Transaction Database | PostgreSQL |
| Analytics Database | ClickHouse |
| Message Queue | NATS JetStream |
| Migration | golang-migrate |
| Authentication | Secure Cookie หรือ JWT ตามรูปแบบ Deployment |
| API Specification | OpenAPI |
| Observability | OpenTelemetry |
| Metrics | Prometheus |
| Dashboard | Grafana |
| Testing | Go testing, Testcontainers |
| Local Environment | Docker Compose |
| CI | GitHub Actions |

---

## Phase 0: Project Foundation

- [ ] สร้าง Go Workspace
- [ ] กำหนด Project Structure
- [ ] ตั้งค่า Configuration
- [ ] ตั้งค่า Structured Logging
- [ ] ตั้งค่า Graceful Shutdown
- [ ] สร้าง Health Check
- [ ] สร้าง Readiness Check
- [ ] ตั้งค่า PostgreSQL
- [ ] ตั้งค่า ClickHouse
- [ ] ตั้งค่า NATS JetStream
- [ ] สร้าง Docker Compose
- [ ] ตั้งค่า Database Migration
- [ ] ตั้งค่า sqlc
- [ ] ตั้งค่า OpenAPI
- [ ] ตั้งค่า Unit Test
- [ ] ตั้งค่า Integration Test
- [ ] ตั้งค่า GitHub Actions

### Definition of Done

- [ ] API รันผ่าน Docker Compose ได้
- [ ] เชื่อมต่อ PostgreSQL, ClickHouse และ NATS ได้
- [ ] Migration ทำงานได้
- [ ] Health Check แสดงสถานะ Dependency
- [ ] CI Build และ Test ผ่าน

---

## Phase 1: Authentication และ Core Domain

### Authentication

- [ ] ออกแบบ User Session
- [ ] Login
- [ ] Logout
- [ ] Refresh Session
- [ ] Authentication Middleware
- [ ] Authorization Middleware
- [ ] CSRF Protection หากใช้ Cookie
- [ ] Secure Cookie Configuration

### Core Domain

- [ ] User
- [ ] Organization
- [ ] Organization Member
- [ ] GitHub Installation
- [ ] Repository
- [ ] Repository Access
- [ ] Sync Job
- [ ] Sync Checkpoint
- [ ] Audit Log

### API

- [ ] `GET /me`
- [ ] `GET /organizations`
- [ ] `GET /organizations/{id}`
- [ ] `GET /repositories`
- [ ] `GET /repositories/{id}`
- [ ] `GET /sync-jobs`
- [ ] `POST /sync-jobs/{id}/retry`

---

## Phase 2: GitHub App Integration

### GitHub App Setup

- [ ] สร้าง GitHub App
- [ ] กำหนด Callback URL
- [ ] กำหนด Webhook URL
- [ ] สร้างและจัดเก็บ Private Key
- [ ] ขอ Permission แบบ Read-only เท่าที่จำเป็น
- [ ] กำหนด Webhook Events
- [ ] รองรับ Installation Callback
- [ ] บันทึก Installation ID
- [ ] ดึงรายการ Repository ที่อนุญาต
- [ ] สร้าง `GET /organizations/{organizationId}/github/connection`
- [ ] สร้าง `POST /organizations/{organizationId}/github/installations/start`
- [ ] สร้าง `GET /organizations/{organizationId}/github/installations/callback`
- [ ] สร้าง `GET /organizations/{organizationId}/github/repositories`
- [ ] สร้าง `POST /organizations/{organizationId}/github/repositories/select`
- [ ] นิยาม frontend state `not_connected`, `installation_required`, `connected`, `syncing`, `sync_failed`

### Permission ขั้นต้น

- [ ] Metadata: Read
- [ ] Contents: Read
- [ ] Pull requests: Read
- [ ] Actions: Read
- [ ] Deployments: Read
- [ ] Checks: Read หากต้องใช้ข้อมูล Check Run

### Token Handling

- [ ] สร้าง GitHub App JWT
- [ ] ขอ Installation Access Token
- [ ] Cache Token ตามเวลาหมดอายุ
- [ ] ไม่บันทึก Installation Token แบบถาวร
- [ ] Rotate Private Key ได้
- [ ] แยก Secret ตาม Environment

### GitHub Client

- [ ] Timeout
- [ ] Retry เฉพาะ Error ที่ Retry ได้
- [ ] Exponential Backoff
- [ ] Pagination
- [ ] Rate Limit Tracking
- [ ] Secondary Rate Limit Handling
- [ ] Request Logging แบบไม่เก็บ Secret
- [ ] Conditional Request ด้วย ETag หากเหมาะสม

---

## Phase 3: Initial Sync

### Sync Flow

- [ ] สร้าง Sync Job เมื่อผู้ใช้เลือกเชื่อมต่อ Repository
- [ ] ดึง Repository Metadata
- [ ] ดึง Pull Request
- [ ] ดึง Pull Request Review
- [ ] ดึง Changed Files
- [ ] ดึง Commit
- [ ] ดึง Workflow Run
- [ ] ดึง Deployment
- [ ] Normalize ข้อมูล
- [ ] บันทึก Checkpoint
- [ ] แสดง Progress
- [ ] Resume จาก Checkpoint
- [ ] ยกเลิก Sync ได้
- [ ] Retry Sync ได้
- [ ] คืนสถานะ initial sync ให้ frontend อ่านได้จาก connection/repository state

### Queue Subjects

```text
github.sync.requested
github.sync.repository
github.sync.pull_requests
github.sync.reviews
github.sync.workflows
github.sync.deployments
metrics.calculate
insights.generate
```

### Worker

- [ ] กำหนด Consumer Group
- [ ] จำกัด Worker Concurrency
- [ ] Ack เมื่อบันทึกข้อมูลสำเร็จ
- [ ] Nak หรือ Retry เมื่อเกิด Temporary Error
- [ ] Dead-letter Subject
- [ ] Job Timeout
- [ ] Idempotency Key
- [ ] Structured Error

---

## Phase 4: GitHub Webhook

### Endpoint

- [ ] `POST /webhooks/github`
- [ ] ตรวจสอบ `X-Hub-Signature-256`
- [ ] อ่าน `X-GitHub-Delivery`
- [ ] อ่าน `X-GitHub-Event`
- [ ] ตอบ GitHub ให้เร็ว
- [ ] บันทึก Delivery ID ป้องกัน Event ซ้ำ
- [ ] ส่ง Event เข้า NATS
- [ ] เก็บ Raw Payload ตาม Data Retention

### Events

- [ ] `installation`
- [ ] `installation_repositories`
- [ ] `pull_request`
- [ ] `pull_request_review`
- [ ] `push`
- [ ] `workflow_run`
- [ ] `deployment`
- [ ] `deployment_status`

### Event Processing

- [ ] Normalize Event
- [ ] Upsert ข้อมูล
- [ ] ป้องกัน Event มาถึงผิดลำดับ
- [ ] Re-fetch จาก GitHub เมื่อ Payload ไม่พอ
- [ ] Trigger Metric Recalculation
- [ ] บันทึก Processing Status
- [ ] Retry Event ที่ล้มเหลว

---

## Phase 5: Data Storage

### PostgreSQL

- [ ] users
- [ ] organizations
- [ ] organization_members
- [ ] github_installations
- [ ] repositories
- [ ] sync_jobs
- [ ] sync_checkpoints
- [ ] webhook_deliveries
- [ ] audit_logs
- [ ] metric_definitions
- [ ] insight_statuses

### ClickHouse

- [ ] pull_request_events
- [ ] review_events
- [ ] commit_events
- [ ] workflow_events
- [ ] deployment_events
- [ ] file_change_events
- [ ] repository_daily_metrics
- [ ] pull_request_metrics
- [ ] reviewer_daily_metrics
- [ ] hotspot_metrics

### Data Concern

- [ ] กำหนด Primary Key และ Ordering Key ให้เหมาะกับ Query
- [ ] ออกแบบ Partition ตามเวลา
- [ ] รองรับ Event ซ้ำ
- [ ] รองรับ Event ที่มาช้า
- [ ] กำหนด Data Retention
- [ ] กำหนดวิธี Rebuild Metric
- [ ] หลีกเลี่ยง Update จำนวนมากใน ClickHouse
- [ ] ใช้ Batch Insert

---

## Phase 6: Metric Engine

### Metric Definitions

- [ ] PR Cycle Time
- [ ] Review Wait Time
- [ ] Review Time
- [ ] Merge Time
- [ ] PR Size
- [ ] Review Coverage
- [ ] Deployment Frequency
- [ ] Change Failure Rate
- [ ] Code Churn
- [ ] Hotspot Score
- [ ] Workload Distribution

### Calculation

- [ ] กำหนดสูตร Metric ในเอกสาร
- [ ] กำหนด Timezone
- [ ] กำหนด Business Day หรือ Calendar Day
- [ ] จัดการ Draft PR
- [ ] จัดการ PR ที่ปิดโดยไม่ Merge
- [ ] จัดการ Bot Account
- [ ] จัดการ Reopened PR
- [ ] จัดการ Force Push
- [ ] คำนวณ Incremental
- [ ] Recalculate ตาม Date Range
- [ ] Version สูตร Metric

### Aggregation

- [ ] Daily Aggregate
- [ ] Weekly Aggregate
- [ ] Repository Aggregate
- [ ] Organization Aggregate
- [ ] Materialized View
- [ ] Backfill Metric
- [ ] Rebuild Aggregate

---

## Phase 7: Dashboard API

### Endpoints

- [ ] `GET /dashboard/summary`
- [ ] `GET /dashboard/pr-cycle-time`
- [ ] `GET /dashboard/review-wait-time`
- [ ] `GET /dashboard/deployments`
- [ ] `GET /dashboard/review-coverage`
- [ ] `GET /dashboard/hotspots`
- [ ] `GET /dashboard/review-queue`
- [ ] `GET /pull-requests`
- [ ] `GET /pull-requests/{id}`
- [ ] `GET /repositories/{id}/metrics`
- [ ] `GET /repositories/{id}/hotspots`
- [ ] `GET /insights`

### API Concern

- [ ] Server-side Pagination
- [ ] Filtering
- [ ] Sorting
- [ ] Date Range Validation
- [ ] Timezone Handling
- [ ] API Versioning
- [ ] Request ID
- [ ] Rate Limiting
- [ ] Response Cache
- [ ] Consistent Error Response
- [ ] OpenAPI Documentation

---

## Phase 8: Insight Engine

- [ ] Slow Review Rule
- [ ] Large PR Rule
- [ ] Hotspot Rule
- [ ] Review Concentration Rule
- [ ] Deployment Failure Rule
- [ ] Rule Configuration
- [ ] Insight Severity
- [ ] Insight Evidence
- [ ] Deduplicate Insight
- [ ] Mark as Reviewed
- [ ] Dismiss Insight
- [ ] Reopen Insight เมื่อปัญหาเกิดซ้ำ

---

## Phase 9: Observability และ Operations

### Logging

- [ ] Structured JSON Log
- [ ] Request ID
- [ ] Trace ID
- [ ] Job ID
- [ ] Repository ID
- [ ] ห้าม Log Token หรือ Private Key

### Metrics

- [ ] HTTP Request Duration
- [ ] HTTP Error Rate
- [ ] Queue Lag
- [ ] Worker Processing Time
- [ ] Worker Failure Rate
- [ ] GitHub Rate Limit Remaining
- [ ] Sync Duration
- [ ] Webhook Processing Delay
- [ ] Database Query Duration

### Tracing

- [ ] API Trace
- [ ] GitHub Request Trace
- [ ] NATS Publish/Consume Trace
- [ ] PostgreSQL Trace
- [ ] ClickHouse Trace

### Grafana

- [ ] API Dashboard
- [ ] Worker Dashboard
- [ ] GitHub Rate Limit Dashboard
- [ ] Sync Dashboard
- [ ] Alert Rule

---

## Phase 10: Security, Testing และ Deployment

### Security

- [ ] Webhook Signature Verification
- [ ] Least Privilege Permission
- [ ] Secret Rotation
- [ ] Encrypt Secret at Rest
- [ ] Input Validation
- [ ] SQL Injection Protection
- [ ] CSRF Protection
- [ ] CORS Configuration
- [ ] Rate Limiting
- [ ] Audit Log
- [ ] Dependency Scanning
- [ ] Container Scanning

### Testing

- [ ] Unit Test Domain Logic
- [ ] Unit Test Metric Formula
- [ ] Integration Test PostgreSQL
- [ ] Integration Test ClickHouse
- [ ] Integration Test NATS
- [ ] GitHub Client Test
- [ ] Webhook Signature Test
- [ ] Duplicate Webhook Test
- [ ] Out-of-order Event Test
- [ ] Initial Sync Resume Test
- [ ] API Contract Test
- [ ] Load Test Dashboard API
- [ ] Load Test Webhook
- [ ] Failure Recovery Test

### Deployment

- [ ] Multi-stage Dockerfile
- [ ] Docker Compose สำหรับ Local
- [ ] Environment Validation
- [ ] Database Migration Pipeline
- [ ] GitHub Actions Build
- [ ] GitHub Actions Test
- [ ] GitHub Actions Security Scan
- [ ] Image Versioning
- [ ] Rollback Plan
- [ ] Backup PostgreSQL
- [ ] Backup Configuration
- [ ] Restore Test

---

## Suggested Folder Structure

```text
cmd/
├── api/
├── worker/
└── migrate/

internal/
├── auth/
├── organization/
├── repository/
├── githubapp/
├── webhook/
├── sync/
├── metrics/
├── insights/
├── dashboard/
├── platform/
│   ├── postgres/
│   ├── clickhouse/
│   ├── nats/
│   ├── telemetry/
│   └── logging/
└── shared/

db/
├── migrations/
├── queries/
└── clickhouse/

api/
└── openapi.yaml

deployments/
└── docker-compose.yml
```

---

## Concerns สำคัญ

### GitHub Rate Limit

- ต้องติดตาม Rate Limit แยกตาม Installation
- Initial Sync ต้องควบคุม Concurrency
- เมื่อใกล้ถึง Limit ต้องชะลอ Job
- Dashboard ต้องอ่านจาก Database ไม่เรียก GitHub โดยตรง

### Duplicate และ Out-of-order Events

- GitHub อาจส่ง Event ซ้ำ
- Event อาจมาถึงไม่เรียงตามเวลา
- ใช้ Delivery ID, Event Timestamp และ Upsert
- Worker ทุกตัวต้องทำงานแบบ Idempotent

### Metric Accuracy

- ต้องกำหนดสูตรให้ชัดเจน
- ต้องแยก Draft, Closed และ Merged PR
- ต้องรองรับ Bot และ Automated Review
- เมื่อสูตรเปลี่ยนต้องรู้ว่า Metric ถูกคำนวณด้วย Version ใด

### Data Privacy

- เก็บเฉพาะข้อมูลที่จำเป็น
- หลีกเลี่ยงการเก็บ Source Code
- กำหนด Data Retention
- ผู้ใช้ต้อง Disconnect และลบข้อมูลได้

### Cost และ Resource Usage

- ทุกเทคโนโลยีรัน Local ได้ฟรี
- ClickHouse และ Grafana ใช้ RAM มากกว่าระบบ CRUD ทั่วไป
- MVP ควรเริ่มจากข้อมูล Repository จำนวนน้อย
- ต้องตั้ง Limit ของ Worker และ Batch Size
