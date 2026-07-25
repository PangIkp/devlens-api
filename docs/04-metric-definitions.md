
# 04-metric-definitions.md

# Metric Definitions

## จุดประสงค์

เอกสารนี้กำหนดความหมาย สูตร และข้อมูลที่ต้องใช้ในการคำนวณ Metric ของ DevLens

> ไฟล์ที่เกี่ยวข้อง
>
> - อ้างอิงภาพรวมระบบ: `01-business-logic-architecture.md`
> - อ้างอิงหน้าที่แสดงผล: `02-frontend-roadmap.md`
> - อ้างอิง API ที่จะส่งค่า Metric: `06-openapi.yaml` *(จะสร้างภายหลัง)*
> - อ้างอิงโครงสร้างฐานข้อมูล: `05-database-design.dbml` *(จะสร้างภายหลัง)*

---

# Template

ทุก Metric ควรมีหัวข้อดังนี้

- Description
- Formula
- Required Data
- Exclude
- Display
- Related API
- Related Tables

---

# 1. PR Cycle Time

## Description

ระยะเวลาตั้งแต่เปิด Pull Request จน Merge สำเร็จ

## Formula

```
merged_at - created_at
```

## Required Data

- created_at
- merged_at

## Exclude

- Draft PR
- Closed without Merge

## Display

- Average
- Median
- Trend

## Related API

GET /dashboard/summary

GET /dashboard/pr-cycle-time

## Related Tables

- pull_requests

---

# 2. Review Wait Time

## Description

เวลาตั้งแต่ Request Review จนได้รับ Review ครั้งแรก

## Formula

```
first_review_at - review_requested_at
```

## Required Data

- review_requested_at
- first_review_at

## Exclude

- ไม่มี Reviewer
- Draft PR

## Display

- Average
- Trend

## Related API

GET /dashboard/review-wait-time

## Related Tables

- pull_requests
- pull_request_reviews

---

# 3. Review Time

## Description

เวลาที่ Reviewer ใช้ในการ Review หลังจากเริ่ม Review

## Formula

```
review_submitted_at - review_started_at
```

> หาก GitHub ไม่มี review_started_at
> ใช้ review_requested_at แทนใน MVP

## Required Data

- review_requested_at
- review_submitted_at

## Display

- Average
- Trend

## Related Tables

- pull_request_reviews

---

# 4. PR Size

## Description

ขนาดของ Pull Request

## Formula

```
files_changed
+
additions
+
deletions
```

## Required Data

- files_changed
- additions
- deletions

## Display

- Small
- Medium
- Large

## Related Tables

- pull_requests

---

# 5. Deployment Frequency

## Description

จำนวน Deployment ที่สำเร็จต่อวัน

## Formula

```
COUNT(successful deployment)
```

## Group By

- Day
- Week
- Month

## Related Tables

- deployments

---

# 6. Change Failure Rate

## Description

เปอร์เซ็นต์ Deployment ที่ล้มเหลว

## Formula

```
failed deployments
/
total deployments
× 100
```

## Display

Percentage

## Related Tables

- deployments

---

# 7. Review Coverage

## Description

เปอร์เซ็นต์ของ Pull Request ที่มี Review อย่างน้อยหนึ่งครั้ง

## Formula

```
reviewed PR
/
total PR
× 100
```

## Related Tables

- pull_requests
- pull_request_reviews

---

# 8. Hotspot Score

## Description

คะแนนความเสี่ยงของไฟล์ที่ถูกแก้ไขบ่อย

## MVP Formula

```
(Number of Changes × Weight)
+
(Code Churn × Weight)
```

> สูตรนี้สามารถปรับปรุงใน Phase 3 ได้

## Required Data

- file_path
- additions
- deletions
- commit_count

## Display

Top 10 Hotspot Files

## Related Tables

- file_changes
- commits

---

# หมายเหตุสำหรับ Phase ถัดไป

## ใช้ใน Database Design

ไฟล์

```
05-database-design.dbml
```

ต้องสามารถเก็บข้อมูลที่ Metric ทุกตัวต้องใช้

เช่น

- merged_at
- review_requested_at
- additions
- deletions
- deployment_status

---

## ใช้ใน API Design

ไฟล์

```
06-openapi.yaml
```

จะต้องมี Endpoint สำหรับส่ง Metric เหล่านี้

เช่น

- GET /dashboard/summary
- GET /dashboard/pr-cycle-time
- GET /dashboard/review-wait-time
- GET /dashboard/deployments

---

## ใช้ใน Frontend

ไฟล์

```
02-frontend-roadmap.md
```

Dashboard Card และ Chart ทุกตัวต้องอ้างอิง Metric ในเอกสารนี้

ห้ามสร้าง Metric ใหม่โดยไม่มีการเพิ่ม Definition ในไฟล์นี้
