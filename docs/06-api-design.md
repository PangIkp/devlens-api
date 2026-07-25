# 06-api-design.md

## Summary

เอกสารนี้กำหนด **แนวทางการออกแบบ API (API Design Guideline)** ของ DevLens

เอกสารนี้ **ไม่ใช่ API Contract** และจะไม่อธิบาย Request/Response ของทุก
Endpoint

รายละเอียดของ API จริง ให้ใช้ **Swagger / OpenAPI** เป็น Source of Truth

------------------------------------------------------------------------

# Related Documents

-   01-business-logic-architecture.md
-   03-backend-roadmap.md
-   04-metric-definitions.md
-   05-database-design.dbml

------------------------------------------------------------------------

# Objectives

-   กำหนดมาตรฐานการออกแบบ API
-   ลดความไม่สอดคล้องระหว่าง Backend และ Frontend
-   ทำให้ API มีรูปแบบเดียวกันทั้งระบบ
-   ใช้ Swagger เป็นเอกสารอ้างอิงหลักสำหรับการพัฒนา

------------------------------------------------------------------------

# API Versioning

ใช้ URL Versioning

    /api/v1

ตัวอย่าง

    GET /api/v1/repositories

------------------------------------------------------------------------

# Authentication

ทุก Endpoint (ยกเว้น Login และ Health Check)

-   ใช้ Bearer Token
-   ส่งผ่าน Authorization Header

```{=html}
<!-- -->
```
    Authorization: Bearer <access_token>

------------------------------------------------------------------------

# Authorization

ใช้ Role-Based Access Control (RBAC)

ตัวอย่าง Role

-   Owner
-   Admin
-   Maintainer
-   Viewer

------------------------------------------------------------------------

# Resource Naming

ใช้คำนามพหูพจน์

ตัวอย่าง

    /organizations
    /repositories
    /sync-jobs
    /metrics

หลีกเลี่ยง

    /getRepositories
    /createRepository

------------------------------------------------------------------------

# HTTP Methods

  Method   Purpose
  -------- ------------------
  GET      Read
  POST     Create / Execute
  PUT      Replace
  PATCH    Partial Update
  DELETE   Remove

------------------------------------------------------------------------

# Pagination

ใช้ Query Parameters

    ?page=1
    &pageSize=20

------------------------------------------------------------------------

# Sorting

    ?sortBy=createdAt
    &sortOrder=desc

------------------------------------------------------------------------

# Filtering

ตัวอย่าง

    ?status=completed
    ?author=min
    ?from=2026-01-01
    ?to=2026-01-31

------------------------------------------------------------------------

# Response Convention

Response ควรมีรูปแบบเดียวกันทั้งระบบ

Success

``` json
{
  "data": {}
}
```

Error

``` json
{
  "error": {
    "code": "REPOSITORY_NOT_FOUND",
    "message": "Repository not found"
  }
}
```

------------------------------------------------------------------------

# Error Handling

ใช้ HTTP Status Code ตามมาตรฐาน

  Code   Meaning
  ------ -----------------------
  200    Success
  201    Created
  204    No Content
  400    Bad Request
  401    Unauthorized
  403    Forbidden
  404    Not Found
  409    Conflict
  422    Validation Error
  500    Internal Server Error

------------------------------------------------------------------------

# Main Resources

Core

-   Organizations
-   Repositories

Operations

-   Sync Jobs
-   Webhook Deliveries

Analytics

-   Pull Requests
-   Reviews
-   Deployments
-   Hotspot Files
-   Metrics

------------------------------------------------------------------------

# API Overview

ตัวอย่าง Resource

    GET    /organizations
    GET    /repositories
    POST   /repositories/{repositoryId}/sync
    GET    /repositories/{repositoryId}/sync-jobs

    GET    /repositories/{repositoryId}/dashboard
    GET    /repositories/{repositoryId}/metrics/pull-requests
    GET    /repositories/{repositoryId}/metrics/reviews
    GET    /repositories/{repositoryId}/metrics/deployments
    GET    /repositories/{repositoryId}/metrics/hotspots

รายละเอียด Endpoint ทั้งหมดอยู่ใน Swagger / OpenAPI

------------------------------------------------------------------------

# Swagger

Swagger / OpenAPI เป็น Source of Truth สำหรับ

-   Endpoints
-   Parameters
-   Request Body
-   Response Body
-   Examples
-   Status Codes
-   Schemas

หากข้อมูลในเอกสารนี้และ Swagger ไม่ตรงกัน ให้ยึด Swagger เป็นหลัก

------------------------------------------------------------------------

# Future Enhancements

-   API Rate Limiting
-   API Version Deprecation
-   Idempotency
-   Request Tracing
-   API Key Support
-   Webhook APIs
