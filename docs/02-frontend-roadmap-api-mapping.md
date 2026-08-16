# 02 - Frontend Roadmap API Mapping

เอกสารนี้สรุปการ map ระหว่าง phase ของ frontend กับ backend API ที่มีอยู่จริงในปัจจุบัน

อัปเดตล่าสุด: `2026-08-13`

## จุดยืนของเอกสาร

- เอกสารนี้ใช้เพื่อช่วยวางแผนงาน frontend
- endpoint, request, response, enum และ schema ให้ยึด `docs/openapi.yaml` เป็น source of truth
- ถ้ามีข้อความในเอกสารนี้ขัดกับ `openapi.yaml` ให้ถือว่า `openapi.yaml` ถูกต้องกว่า
- frontend ไม่ควรเรียก GitHub API โดยตรงใน flow ปกติ แต่ต้องคุยผ่าน backend

## ภาพรวม Current Scope

backend ปัจจุบันพร้อมสำหรับ frontend ในก้อนหลักต่อไปนี้:

- auth และ user session
- organizations และ members
- GitHub App connection flow
- accessible repositories และ repository selection
- repository sync jobs
- dashboard และ metrics
- pull request detail
- insights และ insight review actions

สิ่งที่ยังไม่ใช่ public frontend API ใน milestone ปัจจุบัน:

- dedicated `Disconnect GitHub` endpoint
- public CRUD สำหรับ metric configuration
- public CRUD สำหรับ insight rule configuration
- data retention settings API

## Phase 1: Frontend Foundation

### เป้าหมาย

- ตั้งค่า React + Vite
- TypeScript
- routing
- state management
- query/caching layer
- UI system
- OpenAPI generated client

### Backend APIs

- ยังไม่จำเป็นต้องใช้ API เฉพาะใน phase นี้

### หมายเหตุ

- ควร generate client จาก `docs/openapi.yaml` ตั้งแต่ต้น
- ควรกำหนด error handling กลางให้รองรับรูปแบบ `{ error: { code, message, requestId, details? } }`

## Phase 2: Authentication And Session

### ฟีเจอร์

- login
- refresh session
- logout
- โหลด current user

### Backend APIs

- `POST /auth/login`
- `POST /auth/refresh`
- `POST /auth/logout`
- `GET /me`

### Backend Dependencies

- auth/session flow
- bearer token handling

## Phase 3: Organization And Member Setup

### ฟีเจอร์

- list organizations
- create organization
- organization detail
- update organization
- delete organization
- member management

### Backend APIs

- `POST /organizations`
- `GET /organizations`
- `GET /organizations/{organizationId}`
- `PATCH /organizations/{organizationId}`
- `DELETE /organizations/{organizationId}`
- `GET /organizations/{organizationId}/members`
- `POST /organizations/{organizationId}/members`
- `PATCH /organizations/{organizationId}/members/{memberId}`
- `DELETE /organizations/{organizationId}/members/{memberId}`

### Backend Dependencies

- organization service
- organization member service
- role-based authorization

## Phase 4: GitHub Connection

### ฟีเจอร์

- ดูสถานะ GitHub connection
- เริ่ม GitHub App installation
- รับ callback หลังติดตั้ง
- แสดง installation status

### Backend APIs

- `GET /organizations/{organizationId}/github/connection`
- `POST /organizations/{organizationId}/github/installations/start`
- `GET /organizations/{organizationId}/github/installations/callback`

### Backend Dependencies

- GitHub App credentials
- installation state management

### หมายเหตุ

- phase นี้ไม่มี dedicated `disconnect` endpoint
- ถ้าต้องแสดงสถานะใน UI ให้ยึด state จาก backend เช่น `not_connected`, `installation_required`, `connected`, `syncing`, `sync_failed`

## Phase 5: Repository Onboarding And Sync

### ฟีเจอร์

- list accessible repositories จาก installation
- select repositories ที่จะเชื่อมกับ DevLens
- trigger initial sync
- ดู sync progress
- retry / cancel sync

### Backend APIs

- `GET /organizations/{organizationId}/github/repositories`
- `POST /organizations/{organizationId}/github/repositories/select`
- `POST /organizations/{organizationId}/repositories`
- `GET /organizations/{organizationId}/repositories`
- `GET /repositories/{repositoryId}`
- `PATCH /repositories/{repositoryId}`
- `POST /repositories/{repositoryId}/sync`
- `GET /repositories/{repositoryId}/sync-jobs`
- `GET /sync-jobs/{syncJobId}`
- `POST /sync-jobs/{syncJobId}/retry`
- `POST /sync-jobs/{syncJobId}/cancel`

### Backend Dependencies

- repository discovery
- repository registration/update
- sync job orchestration

### หมายเหตุ

- repository list ใน public contract ปัจจุบันเป็น organization-scoped
- sync job list ใน public contract ปัจจุบันเป็น repository-scoped

## Phase 6: Dashboard And Metrics

### ฟีเจอร์

- dashboard summary
- repository selector
- date range
- metric cards
- review queue
- hotspot files
- workload distribution
- repository metrics overview

### Backend APIs

- `GET /repositories/{repositoryId}/dashboard/summary`
- `GET /repositories/{repositoryId}/dashboard/pr-cycle-time`
- `GET /repositories/{repositoryId}/dashboard/review-wait-time`
- `GET /repositories/{repositoryId}/dashboard/review-queue`
- `GET /repositories/{repositoryId}/metrics`
- `GET /repositories/{repositoryId}/metrics/pull-requests`
- `GET /repositories/{repositoryId}/metrics/reviews`
- `GET /repositories/{repositoryId}/metrics/deployments`
- `GET /repositories/{repositoryId}/metrics/workload-distribution`
- `GET /repositories/{repositoryId}/metrics/hotspots`

### Backend Dependencies

- metrics engine
- PostgreSQL + ClickHouse analytics data

### หมายเหตุ

- `Trend Comparison` ใน milestone ปัจจุบันให้ใช้ trend series จาก metrics endpoints ร่วมกับ `from`, `to`, `interval`
- `Repository Health` ยังไม่แยกเป็น endpoint ใหม่ ให้ derive จาก `dashboard/summary` และ `metrics`

## Phase 7: Pull Request Experience

### ฟีเจอร์

- pull request list
- filters / sorting / pagination
- pull request detail
- review history
- changed files
- timeline
- risk indicator

### Backend APIs

- `GET /pull-requests`
- `GET /pull-requests/{pullRequestId}`

### Backend Dependencies

- pull request repository/service
- synced pull request, review, และ file change data

### หมายเหตุ

- `GET /pull-requests/{pullRequestId}` ปัจจุบันมี `reviews`, `fileChanges`, `timeline`, `riskIndicator`
- การแสดง cycle/review wait ที่ระดับ PR detail ถ้าต้องการเพิ่ม ให้ถือเป็น future enrichment

## Phase 8: Insights

### ฟีเจอร์

- insight list
- filter ตาม organization / repository / type / status / date range
- review insight
- dismiss insight
- reopen insight

### Backend APIs

- `GET /organizations/{organizationId}/insights`
- `GET /insights`
- `POST /organizations/{organizationId}/insights/{insightKey}/review`
- `POST /organizations/{organizationId}/insights/{insightKey}/dismiss`
- `POST /organizations/{organizationId}/insights/{insightKey}/reopen`

### Backend Dependencies

- insight engine
- insight status persistence

### หมายเหตุ

- `GET /insights` เป็น alias ที่ต้องส่ง `organizationId` ใน query
- ถ้าหน้าบ้านมี organization context อยู่แล้ว แนะนำให้ใช้ path หลัก `GET /organizations/{organizationId}/insights`

## Phase 9: Polish And Production Readiness

### เป้าหมาย

- responsive UI
- accessibility
- performance
- loading / error / empty states
- retry behavior
- production readiness

### Backend APIs

- ไม่มี endpoint ใหม่ที่จำเป็นเฉพาะสำหรับ phase นี้

### Backend Dependencies

- endpoint หลักจาก phases ก่อนหน้าต้องถูกใช้งานครบ

## สิ่งที่ frontend ไม่ต้องเรียกโดยตรง

- `POST /github/webhook`
- `POST /webhooks/github`
- `POST /github/webhook-deliveries/{deliveryId}/retry`
- `GET /health`
- `GET /readiness`
- `GET /metrics`

เหตุผล:

- webhook endpoints เป็น integration endpoint สำหรับ GitHub หรือ operator flow
- health / readiness / metrics เป็น operational endpoints ไม่ใช่ user-facing product flow

## ข้อเสนอแนะสำหรับ frontend repo

- generate API client จาก `openapi.yaml` แทนการเขียน types/manual fetch เอง
- แยก model ของ UI ออกจาก raw API response เฉพาะจุดที่จำเป็น
- รองรับ pagination meta ให้สอดคล้องกับ contract ปัจจุบัน
- รองรับ validation error details เพื่อ map ลง form ได้
- เก็บ organization context เป็นแกนหลักของ navigation และ API calls
