# Backend Gap Checklist

สถานะล่าสุดของ backend หลัง recheck เทียบกับ:

- `docs/01-business-logic-architecture.md`
- `docs/03-backend-roadmap.md`
- `docs/04-metric-definitions.md`
- `docs/05-database-design.dbml`
- `docs/06-api-design.md`
- `docs/openapi.yaml`
- implementation ปัจจุบันใน branch นี้

อัปเดตล่าสุด: `2026-08-13`

## 1. Implementation Gaps

- [x] เพิ่ม `GET /repositories/{repositoryId}/metrics/workload-distribution`
- [x] รองรับ `Contributor Distribution` และ `Reviewer Distribution` ด้วย API จริง
- [x] เพิ่ม `timeline` ใน `GET /pull-requests/{id}`
- [x] เพิ่ม `riskIndicator` ใน `GET /pull-requests/{id}`
- [x] เพิ่ม test coverage สำหรับ workload distribution handler
- [x] เพิ่ม test coverage สำหรับ PR timeline / risk indicator derivation

## 2. Scope Decisions Closed

- [x] `Trend Comparison` ถือว่ารองรับแล้วผ่าน metric trend series ปัจจุบัน เช่น `cycleTimeTrend`, `waitTimeTrend`, `deploymentTrend` ร่วมกับ `from`, `to`, `interval`
- [x] `Repository Health` ยังไม่แยกเป็น endpoint ใหม่ใน milestone นี้ และให้ derive จาก `GET /repositories/{repositoryId}/metrics` กับ `dashboard/summary`
- [x] `Metric Configuration` ใน milestone นี้ถือเป็น internal/backend-managed configuration ไม่ใช่ public CRUD API
- [x] `Insight Rule Configuration` ใน milestone นี้ถือเป็น internal/env-driven configuration ไม่ใช่ public CRUD API
- [x] `Data Retention` ใน milestone นี้ถือเป็น deployment/operations concern ไม่ expose เป็น frontend API
- [x] `Disconnect GitHub` ใน milestone นี้ใช้ installation lifecycle/webhook-driven state transition แทน dedicated frontend disconnect endpoint
- [x] metric output อย่าง `median` ที่ยังไม่อยู่ใน `openapi.yaml` ถือเป็น future scope และไม่ใช่ backend blocker สำหรับ frontend รอบนี้

## 3. Validation

- [x] `openapi.yaml` สอดคล้องกับ implementation รอบนี้
- [x] `go test ./...` ผ่าน โดยรันด้วย local Go cache ใน repo
- [x] ไม่มี implementation gap ที่ block frontend integration ตาม public API contract ปัจจุบัน

## 4. Current Conclusion

- [x] Backend baseline สำหรับ frontend พร้อมใช้งานแล้วตาม `docs/openapi.yaml`
- [x] ถ้าจะมีงานต่อจากนี้ จะเป็น `feature expansion` จาก business vision ไม่ใช่ `missing implementation` ของ contract ปัจจุบัน
