# Backend Operations

เอกสารนี้สรุปเฉพาะสิ่งที่จำเป็นต่อการใช้งาน ดูแล และ deploy backend ปัจจุบันของ DevLens

อัปเดตล่าสุด: `2026-08-13`

## 1. Current Status

- backend public API พร้อมใช้งานตาม `docs/openapi.yaml`
- backend ปัจจุบันรองรับ metrics, insights, GitHub App connection, sync, webhook, และ async processing flow หลักแล้ว
- สิ่งที่ยังไม่อยู่ใน `openapi.yaml` ให้ถือเป็น `future scope` ไม่ใช่ operational blocker

## 2. Runtime Dependencies

backend ปัจจุบันต้องพึ่ง:

- PostgreSQL
- ClickHouse
- NATS JetStream

ก่อนถือว่าระบบพร้อมใช้งาน ควรตรวจอย่างน้อย:

- `GET /api/v1/health`
- `GET /api/v1/readiness`
- `GET /metrics`
- migration อยู่ที่ version ที่ถูกต้อง
- ClickHouse schema init สำเร็จ
- NATS stream/subjects พร้อม

## 3. Environment Profiles

- `local`: Docker Compose + `.env`
- `staging`: production-like dependencies + staging secrets
- `production`: managed/hardened PostgreSQL, ClickHouse, NATS และใช้ secret manager ของ platform

หลักการร่วมทุก environment:

- secret ต้อง inject จาก environment หรือ secret store
- ห้าม commit `.env`
- แยก credentials ของแต่ละ environment ออกจากกัน

## 4. Deployment Flow

ลำดับ deploy ที่ควรยึด:

1. build artifact หรือ image จาก commit ที่ชัดเจน
2. provision PostgreSQL, ClickHouse, NATS
3. inject env และ secrets
4. run PostgreSQL migrations
5. start หรือ roll backend
6. ตรวจ health, readiness, metrics, และ async pipeline

หลัง deploy ควรเช็กเพิ่ม:

- sync jobs ยังเดินต่อได้
- webhook retry backlog ไม่ค้าง
- metrics calculation ยัง trigger ได้
- insights generation ยังอัปเดตสถานะได้
- metrics/insights endpoint ของ repository ตัวอย่างตอบได้จริง

## 5. Backup And Restore

### PostgreSQL

ใช้ commands:

```sh
make backup-postgres
BACKUP_FILE=/absolute/path/to/backup.dump make restore-postgres
BACKUP_FILE=/absolute/path/to/backup.dump make verify-postgres-restore
```

ใช้ `verify-postgres-restore` สำหรับทดสอบ restore ลง temporary database ก่อนใช้งานจริง

### ClickHouse

ถ้าเป็น local/self-hosted ให้ใช้ volume หรือ filesystem snapshot

restore flow:

1. หยุด backend ที่เขียนเข้า ClickHouse
2. restore snapshot/volume
3. start ClickHouse
4. รัน `make data-cleanup` หากต้อง re-apply retention
5. รัน `make metrics-rebuild` หาก aggregate ไม่สอดคล้อง

หลัง restore ควรเช็ก:

- ClickHouse healthy
- health endpoint รายงาน `clickhouse.status = ok`
- metrics endpoints ของ repository ตัวอย่างยังตอบได้

## 6. Data Retention

retention policy ปัจจุบัน:

- raw analytics ใน ClickHouse ใช้ `ANALYTICS_RAW_RETENTION_DAYS` default `180`
- aggregate `metrics_daily` ใช้ `ANALYTICS_AGGREGATE_RETENTION_DAYS` default `365`
- `webhook_deliveries.payload` ใช้ `WEBHOOK_PAYLOAD_RETENTION_DAYS` default `30`
- soft-deleted organizations ใช้ `SOFT_DELETED_ORGANIZATION_RETENTION_DAYS` default `30`
- disconnected GitHub installations ใช้ `DISCONNECTED_INSTALLATION_RETENTION_DAYS` default `30`

cleanup command:

```sh
make data-cleanup
```

command นี้จะ:

1. purge webhook payload ที่หมดอายุ
2. purge analytics data ของ disconnected installations
3. purge application state ของ disconnected installations
4. purge analytics data ของ soft-deleted organizations
5. hard delete organizations ที่เกิน retention

หมายเหตุ:

- ClickHouse purge ใช้ mutation แบบ asynchronous
- ถ้า reconnect หลัง installation ถูก purge ไปแล้ว จะถือเป็น onboarding ใหม่และต้อง initial sync ใหม่

## 7. Secret And Security Baseline

backend ปัจจุบันใช้หลักการดังนี้:

- `GITHUB_APP_PRIVATE_KEY` และ `GITHUB_WEBHOOK_SECRET` อ่านจาก environment เท่านั้น
- application ไม่เก็บ secret เหล่านี้ลง PostgreSQL หรือ ClickHouse
- encryption at rest ของ secret ให้พึ่ง deployment platform / secret manager

secret ที่ควรแยก rotation ชัดเจน:

- `GITHUB_APP_PRIVATE_KEY`
- `GITHUB_WEBHOOK_SECRET`
- `POSTGRES_PASSWORD`
- `CLICKHOUSE_PASSWORD`
- NATS credentials

GitHub App permission baseline ปัจจุบัน:

- `metadata: read`
- `pull_requests: read`
- `contents: read`
- `actions: read`
- `deployments: read`

ถ้าสิทธิ์ไม่พอ:

- connection อาจอยู่ใน state `installation_required`
- accessible repositories อาจถูก mark เป็น `permission_missing`

## 8. API Security Notes

- transport ปัจจุบันใช้ bearer token
- ยังไม่มี cookie-based browser session
- CSRF จึงยังไม่ใช่ requirement ของ transport ปัจจุบัน
- ถ้าอนาคตเปลี่ยนเป็น cookie auth ต้องเพิ่ม CSRF protection และ origin validation

backend ปัจจุบันรองรับ `ETag` / `If-None-Match` ใน read endpoints ที่เหมาะกับ cache validation บางตัว เช่น:

- `GET /api/v1/me`
- `GET /api/v1/organizations/{organizationId}/github/connection`
- `GET /api/v1/organizations/{organizationId}/github/repositories`

## 9. Logging And Observability

backend ปัจจุบันมี:

- structured logs
- Prometheus metrics endpoint
- OpenTelemetry support ผ่าน OTLP env
- local Grafana/Prometheus stack สำหรับ development

ค่าที่สำคัญ:

- `OTEL_ENABLED=true`
- `OTEL_EXPORTER_OTLP_ENDPOINT=<host:port>`
- optional: `OTEL_SERVICE_NAME`, `OTEL_EXPORTER_OTLP_INSECURE`, `OTEL_TRACE_SAMPLE_RATIO`

หลักการ log:

- ห้าม log token, private key, webhook secret, authorization header
- panic recovery ต้อง redact secret-like values

## 10. Backend Completion Note

backend ถือว่า complete สำหรับ current milestone ถ้ายึด `docs/openapi.yaml` เป็น source of truth

งานที่ยังไม่ได้อยู่ใน public API contract ปัจจุบัน ให้ถือเป็น future scope ไม่ใช่ missing backend implementation
