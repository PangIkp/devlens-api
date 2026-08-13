# 04-metric-definitions.md

# Metric Definitions

## จุดประสงค์

เอกสารนี้กำหนดความหมาย สูตร ข้อมูลที่ต้องใช้ วิธี Aggregate และ API ที่เกี่ยวข้องกับ Metric ของ DevLens

เอกสารนี้ใช้ร่วมกับ:

- `01-business-logic-architecture.md`
- `03-backend-roadmap.md`
- `05-database-design.dbml`
- `06-api-design.md`
- `openapi.yaml`

> หมายเหตุ: `openapi.yaml` เป็น Source of Truth สำหรับ API Contract  
> ถ้าชื่อ Endpoint ในเอกสารนี้ไม่ตรงกับ OpenAPI ให้ยึด `openapi.yaml`

---

# Metric Design Principles

- Metric ใช้เพื่อวิเคราะห์ **process และ system health** ไม่ใช่จัดอันดับ Developer
- Dashboard อ่านค่าจากข้อมูลที่ Sync และคำนวณไว้แล้ว ไม่เรียก GitHub เพื่อคำนวณใหม่ทุกครั้ง
- Metric ต้องรองรับการ Recalculate เมื่อสูตรเปลี่ยนหรือมีข้อมูลย้อนหลังเข้ามา
- Metric ที่ใช้ค่าเฉลี่ยหลายวันต้อง Aggregate ด้วย sample/count ที่ถูกต้อง เพื่อหลีกเลี่ยง average-of-averages
- ต้องรองรับ Duplicate Event และ Out-of-order Event จาก GitHub
- สูตร Metric ต้องมี Version เพื่อให้รู้ว่าค่าที่แสดงถูกคำนวณด้วยนิยามใด
- Metric ที่เป็นค่า "ต่อวัน" ต้องระบุชัดว่าใช้ `calendar day` หรือ `business day`

## Calendar Day vs Business Day

- `calendar day` = ทุกวันในช่วงที่เลือก รวมเสาร์อาทิตย์
- `business day` = วันจันทร์ถึงศุกร์ในช่วงที่เลือก โดยยังไม่หัก public holidays ใน MVP ปัจจุบัน
- Metric ที่เป็น elapsed duration เช่น `PR Cycle Time`, `Review Wait Time`, `Review Time` ยังใช้เวลาจริงตาม timestamp ไม่แปลงเป็น business minutes
- Metric ที่ normalize เป็นค่า "ต่อวัน" เช่น `Deployment Frequency` สามารถคำนวณได้ทั้งแบบ `calendar` และ `business`
- หาก client ไม่ส่งพารามิเตอร์เพิ่ม backend จะใช้ค่า default จาก server-side metric rules

---

# Metric Specification Template

Metric แต่ละตัวควรระบุ:

- Description
- Status
- Unit
- Formula
- Required Data
- Exclude / Edge Cases
- Aggregation
- Display
- Refresh Trigger
- Related API
- Related Data
- Notes

---

# Core Metrics

## 1. PR Cycle Time

### Description

ระยะเวลาตั้งแต่เปิด Pull Request จน Merge สำเร็จ

### Status

MVP

### Unit

Minutes

### Formula

```text
merged_at - created_at
```

### Required Data

- Pull Request ID
- Repository ID
- `created_at`
- `merged_at`
- PR state
- Draft state

### Exclude / Edge Cases

- Draft PR
- PR ที่ปิดโดยไม่ Merge
- PR ที่ยังไม่ Merge ไม่มีค่า PR Cycle Time

กรณี Reopened PR ต้องเก็บ event/state ให้เพียงพอสำหรับ Recalculate ตามนิยาม Metric Version ที่ใช้งาน

### Aggregation

รองรับอย่างน้อย:

- Average
- Median
- Trend ตามช่วงเวลา

หาก Aggregate จาก Daily Metrics หลายวัน ห้ามเฉลี่ยค่า Average รายวันตรง ๆ ต้องใช้ sample/count ที่เกี่ยวข้อง

### Display

- Dashboard Summary
- Pull Request Metric Trend
- Repository Detail

### Refresh Trigger

- Initial Sync
- Pull Request created/updated/merged event
- Manual Recalculate
- Backfill/Rebuild

### Related API

- `GET /repositories/{repositoryId}/dashboard/summary`
- `GET /repositories/{repositoryId}/metrics/pull-requests`

### Related Data

- Pull Request source/event data
- Pull Request aggregate metrics
- Repository daily metrics

---

## 2. Review Wait Time

### Description

เวลาตั้งแต่ Request Review จนได้รับ Review แรก

### Status

MVP

### Unit

Minutes

### Formula

```text
first_review_at - review_requested_at
```

### Required Data

- Pull Request ID
- `review_requested_at`
- `first_review_at`
- Reviewer
- PR draft/state

### Exclude / Edge Cases

- PR ที่ไม่มี Reviewer
- PR ที่ยังไม่ได้รับ Review
- Draft PR

Bot และ Automated Review ต้องสามารถแยกได้ตามข้อมูลที่ระบบเก็บ เพื่อรองรับการปรับสูตรในอนาคต

### Aggregation

- Average
- Trend ตามช่วงเวลา

หาก Aggregate หลายวัน ต้องใช้ `review_wait_sample_count` หรือข้อมูลจำนวน sample ที่เทียบเท่าในการถ่วงน้ำหนัก

### Display

- Dashboard Summary
- Review Metrics
- Review Wait Time Trend

### Refresh Trigger

- Initial Sync
- Review requested
- Review submitted
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/dashboard/summary`
- `GET /repositories/{repositoryId}/metrics/reviews`

### Related Data

- Pull Request Review source/event data
- Review aggregate metrics
- Repository daily metrics

---

## 3. Review Time

### Description

เวลาที่ใช้ในการ Review Pull Request

### Status

MVP

### Unit

Minutes

### Formula

แนวคิดเดิมของเอกสารคือ:

```text
review_submitted_at - review_started_at
```

อย่างไรก็ตาม หาก GitHub ไม่มี `review_started_at` ที่น่าเชื่อถือสำหรับ MVP ให้ใช้ approximation:

```text
review_submitted_at - review_requested_at
```

### Required Data

- Pull Request ID
- `review_requested_at`
- `review_submitted_at`
- Reviewer

### Exclude / Edge Cases

- ไม่มี Review
- Timestamp ไม่ครบ
- Automated Review/Bot ควรแยกได้เพื่อรองรับกติกาในอนาคต

### Aggregation

- Average
- Trend

หาก Aggregate หลายวันต้องใช้ `review_time_sample_count` หรือจำนวน sample ที่เทียบเท่า

### Display

- Review Metrics
- Repository Detail

### Refresh Trigger

- Review submitted
- Initial Sync
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/metrics/reviews`

### Related Data

- Review source/event data
- Review aggregate metrics
- Repository daily metrics

### Notes

นิยาม Review Time ยังมีข้อจำกัดจาก timestamp ที่ GitHub ให้มา จึงควร Version สูตรนี้เมื่อมีการเปลี่ยนวิธีคำนวณ

---

## 4. PR Size

### Description

ขนาดของ Pull Request จากจำนวนไฟล์และจำนวนบรรทัดที่เปลี่ยน

### Status

MVP

### Unit

- Files changed
- Additions
- Deletions

### Formula

PR Size ไม่ควรถูกรวมเป็นเลขเดียวโดยไม่มีบริบท สำหรับ MVP ให้เก็บและแสดงองค์ประกอบแยก:

```text
files_changed
additions
deletions
```

### Required Data

- `files_changed`
- `additions`
- `deletions`

### Exclude / Edge Cases

- ข้อมูล Changed Files ไม่ครบ
- Force Push อาจทำให้ค่าที่ Sync ก่อนหน้าเปลี่ยน ต้องรองรับการ Re-fetch/Recalculate

### Aggregation

- Average Files Changed
- Average Additions
- Average Deletions

หาก Aggregate หลายวันต้องถ่วงน้ำหนักด้วยจำนวน PR ที่ใช้เป็น sample

### Display

- Pull Request Metrics
- Repository Trend
- Large PR Insight

### Refresh Trigger

- Initial Sync
- Pull Request update
- Push/Force Push ที่กระทบ PR
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/metrics/pull-requests`

### Related Data

- Pull Request source/event data
- File change event data
- Pull Request aggregate metrics
- Repository daily metrics

---

## 5. Deployment Frequency

### Description

จำนวน Deployment ที่สำเร็จในช่วงเวลาที่กำหนด

### Status

MVP / ขึ้นกับ Deployment ingestion

### Unit

Count per period

### Formula

```text
COUNT(successful deployment)
```

### Required Data

- Repository ID
- Deployment ID
- Deployment status
- `deployed_at`
- Environment

### Exclude / Edge Cases

- Deployment ที่ไม่สำเร็จไม่นับใน Deployment Frequency
- Event ซ้ำต้องไม่เพิ่ม Count ซ้ำ
- Event ที่มาช้าต้องสามารถ Recalculate ช่วงเวลาที่ได้รับผลกระทบได้

### Aggregation

- Daily
- Weekly
- Monthly
- Date Range

การรวมหลายวันให้ใช้จำนวน successful deployment จริง (`SUM(successful_deployment_count)` หรือข้อมูลเทียบเท่า)

### Display

- Dashboard Summary
- Deployment Metrics
- Deployment Trend

### Refresh Trigger

- Deployment / Deployment Status event
- Initial Sync / Backfill
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/dashboard/summary`
- `GET /repositories/{repositoryId}/metrics/deployments`

### Related Data

- Deployment source/event data
- Deployment aggregate metrics
- Repository daily metrics

---

## 6. Change Failure Rate

### Description

สัดส่วน Deployment ที่ล้มเหลวเมื่อเทียบกับ Deployment ทั้งหมดในช่วงเวลาที่กำหนด

### Status

MVP / ขึ้นกับ Deployment ingestion

### Unit

Ratio `0..1` สำหรับ API  
Frontend สามารถ Format เป็น Percentage ได้

### Formula

```text
failed_deployments / total_deployments
```

### Required Data

- Deployment status
- Deployment timestamp
- Successful deployment count
- Failed deployment count

### Exclude / Edge Cases

- หากไม่มี Deployment ในช่วงเวลา ให้คืนค่าตาม API contract โดยไม่หารด้วยศูนย์
- Event ซ้ำต้องไม่ถูก Count ซ้ำ

### Aggregation

Date Range ต้องคำนวณจาก Count จริง:

```text
SUM(failed_deployment_count)
/
SUM(total_deployment_count)
```

ห้าม Average ค่า Failure Rate รายวันตรง ๆ

### Display

- Dashboard Summary
- Deployment Metrics

### Refresh Trigger

- Deployment / Deployment Status event
- Initial Sync / Backfill
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/dashboard/summary`
- `GET /repositories/{repositoryId}/metrics/deployments`

### Related Data

- Deployment source/event data
- Deployment aggregate metrics
- Repository daily metrics

---

## 7. Review Coverage

### Description

สัดส่วน Pull Request ที่มี Review อย่างน้อยหนึ่งครั้ง

### Status

MVP

### Unit

Ratio `0..1` สำหรับ API  
Frontend สามารถ Format เป็น Percentage ได้

### Formula

```text
reviewed_pr_count / total_pr_count
```

### Required Data

- Pull Request ID
- Review relation
- `reviewed_pr_count`
- `pr_count`

### Exclude / Edge Cases

- ต้องกำหนดให้สอดคล้องกันว่า Draft PR ถูกนับใน denominator หรือไม่
- Bot/Automated Review ต้องสามารถแยกได้เพื่อรองรับ Metric Version ในอนาคต

สำหรับ MVP ให้ใช้กติกาที่ implementation ปัจจุบันกำหนดไว้ และต้องบันทึกเป็น Metric Version

### Aggregation

Date Range ต้องใช้ Count จริง:

```text
SUM(reviewed_pr_count)
/
SUM(pr_count)
```

ห้าม Average ค่า Review Coverage รายวันตรง ๆ

### Display

- Dashboard Summary
- Review Metrics

### Refresh Trigger

- Pull Request event
- Review event
- Initial Sync
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/dashboard/summary`
- `GET /repositories/{repositoryId}/metrics/reviews`

### Related Data

- Pull Request source/event data
- Review source/event data
- Review aggregate metrics
- Repository daily metrics

---

## 8. Hotspot Score

### Description

คะแนนความเสี่ยงของไฟล์ที่ถูกแก้ไขบ่อยและมี Code Churn สูง

### Status

MVP / สูตรต้องมี Version

### Unit

Score

### MVP Formula

เอกสารเดิมกำหนดแนวคิดไว้ว่า:

```text
(Number of Changes × Weight)
+
(Code Churn × Weight)
```

### Required Data

- `file_path`
- additions
- deletions
- commit count / number of changes
- Repository ID
- Time Range

### Exclude / Edge Cases

- File rename
- Deleted file
- Generated file
- Vendor/dependency file

กติกา exclude สำหรับ generated/vendor file ยังไม่ได้ถูกกำหนดในเอกสารปัจจุบัน จึงต้องระบุใน Metric Version เมื่อ implement

### Aggregation

- Rank ภายใน Repository
- Rank ตาม Date Range
- Top N

### Display

- Hotspot Table
- Repository Detail
- Insight

### Refresh Trigger

- Pull Request changed files
- Push/Commit event
- Initial Sync / Backfill
- Manual Recalculate

### Related API

- `GET /repositories/{repositoryId}/metrics/hotspots`

### Related Data

- File change event data
- Commit event data
- Hotspot aggregate metrics

### Notes

น้ำหนัก (`Weight`) ยังไม่ได้กำหนดค่าตายตัวในเอกสารปัจจุบัน ดังนั้น implementation ต้องผูกกับ `metric_version` และห้ามเปลี่ยนสูตรโดยไม่มี version ใหม่

MVP ปัจจุบันรองรับการปรับน้ำหนักผ่าน server-side metric rules สำหรับ:

- commit count
- additions
- deletions

---

# Extended / Planned Metrics

Metric ต่อไปนี้ถูกระบุใน Business Logic หรือ Backend Roadmap แล้ว แต่ยังไม่มีสูตรครบพอที่จะถือเป็น Source of Truth สำหรับการ implement

---

## 9. Merge Time

### Description

Metric นี้ถูกระบุใน Backend Roadmap

### Status

Planned — Formula TBD

### Formula

ยังไม่ได้กำหนดในเอกสารปัจจุบัน

### Required Data

TBD

### Related API

ยังไม่มี Contract เฉพาะใน OpenAPI ปัจจุบัน

### Notes

ต้องกำหนดก่อน implement ว่า Merge Time ต่างจาก PR Cycle Time อย่างไร เช่น จะเริ่มนับหลัง approval, หลัง review completion หรือ event อื่น

---

## 10. Code Churn

### Description

Metric นี้ถูกระบุใน Backend Roadmap และถูกใช้เป็นส่วนประกอบแนวคิดของ Hotspot Score

### Status

Planned — Formula TBD

### Formula

ยังไม่ได้กำหนดสูตร Source of Truth ในเอกสารปัจจุบัน

### Required Data

ข้อมูลที่คาดว่าจำเป็นจากเอกสารปัจจุบัน:

- additions
- deletions
- file changes
- commit/file change events

### Related API

ยังไม่มี Contract เฉพาะใน OpenAPI ปัจจุบัน

### Notes

ต้องกำหนดก่อน implement ว่า Code Churn จะวัดเป็น `additions + deletions`, absolute changed lines หรือสูตรอื่น

---

## 11. Workload Distribution

### Description

การกระจาย Pull Request และ Review ภายในทีม เพื่อช่วยมองเห็นว่างานหรือ Review กระจุกตัวอยู่ที่สมาชิกบางคนหรือไม่

### Status

Planned

### Principle

ใช้เพื่อวิเคราะห์การกระจายของ process ไม่ใช่จัดอันดับผลงานรายบุคคล

### Formula

ยังไม่ได้กำหนดสูตรตายตัวในเอกสารปัจจุบัน

### Required Data

จาก Business Logic ปัจจุบันอย่างน้อยต้องมี:

- Pull Request author
- Reviewer
- Pull Request count
- Review count
- Time Range
- Repository / Organization scope

### Display

- Contributor Distribution
- Reviewer Distribution
- Concentration Insight

### Related API

ยังไม่มี Contract เฉพาะใน OpenAPI ปัจจุบัน

### Notes

ก่อน implement ต้องกำหนดว่าจะใช้ Count, Share/Percentage, concentration ratio หรือวิธีอื่น และต้องระวังไม่ให้ UI กลายเป็น Developer Ranking

---

# Aggregation Rules

## Daily Metrics

ระบบสามารถเก็บ Daily Aggregate เพื่อให้ Dashboard Query เร็วขึ้น

อย่างไรก็ตาม Metric ที่เป็น Average/Ratio ต้องเก็บ numerator, denominator หรือ sample count ที่เพียงพอสำหรับ Aggregate หลายวันอย่างถูกต้อง

ตัวอย่าง field ที่มีเหตุผลจาก design ปัจจุบัน:

- `pr_count`
- `merged_pr_count`
- `reviewed_pr_count`
- `review_wait_sample_count`
- `review_time_sample_count`
- `successful_deployment_count`
- `failed_deployment_count`

## Multi-day Average

ห้ามทำ:

```text
AVG(daily_average)
```

เมื่อจำนวน sample ของแต่ละวันไม่เท่ากัน

ควรใช้แนวคิด:

```text
SUM(daily_average * daily_sample_count)
/
SUM(daily_sample_count)
```

หรือเก็บ sum + count โดยตรง

## Ratio

Metric เช่น Review Coverage และ Change Failure Rate ต้องคำนวณจาก Count รวมของช่วงเวลา ไม่ใช่ Average ของ Ratio รายวัน

---

# Metric Lifecycle

```text
GitHub
  ↓
Initial Sync / Webhook
  ↓
Normalize Event
  ↓
Queue / Worker
  ↓
Raw / Event Storage
  ↓
Metric Calculation
  ↓
Daily / Repository Aggregate
  ↓
Dashboard API
  ↓
Frontend
```

Dashboard ไม่ควรเรียก GitHub เพื่อคำนวณ Metric แบบ realtime ต่อ request

---

# Refresh Strategy

Metric สามารถถูกคำนวณใหม่จาก:

- Initial Sync
- GitHub Webhook ที่เกี่ยวข้อง
- Incremental Sync
- Manual Recalculate
- Backfill
- Rebuild Aggregate

Scheduled recalculation สามารถเพิ่มในอนาคตได้

Worker ที่คำนวณ Metric ต้องทำงานแบบ Idempotent

---

# Metric Versioning

Backend Roadmap กำหนดให้รองรับ Version สูตร Metric

อย่างน้อยต้องสามารถระบุได้ว่า:

- Metric ชื่ออะไร
- Version ใด
- คำนวณเมื่อไร
- ใช้ช่วงข้อมูลใด

Metadata ที่ควรรองรับใน Data Design:

```text
metric_version
calculated_at
```

หากมี algorithm/config ที่เปลี่ยนตาม version สามารถเพิ่ม metadata ที่เกี่ยวข้องใน Database Design ได้

> ห้ามเปลี่ยนสูตร Metric เดิมแล้วเขียนทับความหมายของ Version เดิมโดยไม่มี Migration/Rebuild Strategy

---

# Data Quality Rules

## Draft Pull Request

Metric ที่เกี่ยวกับ Cycle/Review ต้องกำหนดชัดเจนว่า Draft ถูก exclude หรือไม่

สำหรับ PR Cycle Time และ Review Wait Time ใน MVP ให้ exclude Draft ตาม definition เดิม

## Closed Without Merge

ไม่ใช้ในการคำนวณ PR Cycle Time

แต่สามารถเก็บ source event ไว้สำหรับ analytics อื่นในอนาคตได้

## Bot / Automated Review

Backend Roadmap กำหนดว่าต้องรองรับ Bot Account

ดังนั้น normalized data ควรสามารถแยก Human กับ Bot ได้ แม้ MVP บาง Metric จะยังไม่ได้ใช้ filter นี้

## Reopened Pull Request

ต้องเก็บ event/state เพียงพอให้ Recalculate ได้เมื่อกติกาของ Metric กำหนด

## Force Push

ต้องรองรับกรณีที่ PR Size / Changed Files เปลี่ยนหลัง Force Push และ Recalculate ข้อมูลที่ได้รับผลกระทบ

## Duplicate / Out-of-order Events

Metric Engine ต้องอาศัย ingestion ที่ idempotent และสามารถ rebuild จาก source/event data ได้ เพื่อไม่ให้ Metric เพิ่มซ้ำหรือผิดลำดับ

## Missing Data

หาก Required Data ไม่ครบ:

- อย่าสร้างค่าที่คาดเดาเอง
- exclude sample นั้นจาก Metric ที่คำนวณไม่ได้
- เก็บสถานะ/error ที่ช่วยตรวจสอบ Data Quality ได้เมื่อเหมาะสม

---

# Time and Date Rules

Backend Roadmap ระบุว่าต้องกำหนด:

- Timezone
- Business Day หรือ Calendar Day

แต่เอกสารปัจจุบันยังไม่ได้กำหนดค่า Source of Truth

ดังนั้นก่อน Production ต้องตัดสินใจและบันทึกกติกาเหล่านี้ใน Metric Version / Configuration

สำหรับ API ให้ใช้รูปแบบ Date Range ตาม `openapi.yaml`

---

# API Mapping

OpenAPI ปัจจุบันใช้ Repository-scoped Analytics APIs:

```text
GET /repositories/{repositoryId}/dashboard/summary
GET /repositories/{repositoryId}/metrics/pull-requests
GET /repositories/{repositoryId}/metrics/reviews
GET /repositories/{repositoryId}/metrics/deployments
GET /repositories/{repositoryId}/metrics/hotspots
```

Mapping หลัก:

| API | Metric |
|---|---|
| Dashboard Summary | PR Cycle Time, Review Wait Time, Deployment Frequency, Change Failure Rate, Review Coverage |
| Pull Request Metrics | PR Cycle Time, PR Size |
| Review Metrics | Review Wait Time, Review Time, Review Coverage |
| Deployment Metrics | Deployment Frequency, Change Failure Rate |
| Hotspot Metrics | Hotspot Score |

หาก API Contract เปลี่ยน ให้แก้ `openapi.yaml` ก่อน แล้ว sync เอกสารนี้ตาม

---

# Data Model Mapping

Data Model ฉบับใหม่ต้องรองรับ source/event และ aggregate data ที่ Backend Roadmap ระบุ เช่น:

## Source / Event

- Pull Request Events
- Review Events
- Commit Events
- Workflow Events
- Deployment Events
- File Change Events

## Aggregate

- Repository Daily Metrics
- Pull Request Metrics
- Reviewer Daily Metrics
- Hotspot Metrics

รายละเอียด Table/Column ที่แท้จริงให้ยึด `05-database-design.dbml` หลังอัปเดต

---

# MVP Scope

Metric ที่ถือเป็น Core MVP:

1. PR Cycle Time
2. Review Wait Time
3. Review Time
4. PR Size
5. Deployment Frequency
6. Change Failure Rate
7. Review Coverage
8. Hotspot Score

Metric ที่มีใน Product/Backend Roadmap แต่ยังต้อง finalize definition:

- Merge Time
- Code Churn
- Workload Distribution

---

# Definition of Done สำหรับ Metric ใหม่

ห้ามเพิ่ม Metric ใหม่เข้า Dashboard จนกว่าจะมี:

- Description
- Formula
- Unit
- Required Data
- Exclude / Edge Cases
- Aggregation Rule
- Display
- Refresh Trigger
- Metric Version
- Data Source
- API Mapping หรือสถานะ Planned ที่ชัดเจน
