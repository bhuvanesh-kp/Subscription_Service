# Subscription Service

A full-stack subscription management system built in Go, demonstrating real-world concurrency patterns for handling billing, document generation, and email delivery at scale.

---

## Table of Contents

- [Overview](#overview)
- [Tech Stack](#tech-stack)
- [Project Structure](#project-structure)
- [Database Schema](#database-schema)
- [Getting Started](#getting-started)
- [API Endpoints](#api-endpoints)
- [Concurrency — Design & Significance](#concurrency--design--significance)
- [Future Scope](#future-scope)

---

## Overview

Subscription Service is a web application that allows users to register, verify their email, log in, and subscribe to one of three monthly plans (Bronze, Silver, Gold). Upon subscribing or changing plans, the system concurrently generates a personalized invoice and a PDF user manual, delivering both to the user via email — without blocking the HTTP response.

**Key capabilities:**

- User registration with email-based account activation
- Secure session-based authentication (bcrypt + Redis)
- Subscription plan management with upgrade and downgrade support
- Concurrent invoice generation and PDF manual creation on every plan change
- Async email delivery via a dedicated channel-based worker
- Graceful application shutdown that drains all in-flight goroutines

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.21+ |
| HTTP Router | go-chi/chi v5 |
| Session Store | alexedwards/scs v2 + Redis |
| Database | PostgreSQL 14.2 (via pgx v4) |
| Cache | Redis (Alpine) |
| Email | xhit/go-simple-mail v2 + MailHog (dev) |
| PDF Generation | phpdave11/gofpdf + gofpdi |
| CSS Inlining | vanng822/go-premailer |
| Password Hashing | golang.org/x/crypto (bcrypt, cost 12) |
| URL Signing | bwmarrin/go-alone |
| Frontend | Bootstrap 5.1.3 + SweetAlert2 |
| Containers | Docker Compose |

---

## Project Structure

```
Subscription_Service/
├── cmd/web/
│   ├── main.go              # App bootstrap, goroutine lifecycle, graceful shutdown
│   ├── config.go            # Application-wide Config struct
│   ├── routes.go            # Public and protected route definitions
│   ├── handlers.go          # HTTP request handlers
│   ├── middleware.go        # Session load and Auth middleware
│   ├── mailer.go            # Channel-based async email worker
│   ├── render.go            # Template rendering + TemplateData helpers
│   ├── helper.go            # Shared utility functions
│   ├── signer.go            # Signed URL generation and verification
│   └── templates/           # Go HTML templates (pages, partials, emails)
├── data/
│   ├── models.go            # DB handle and Models container
│   ├── user.go              # User CRUD and password helpers
│   └── plan.go              # Plan queries and subscription logic
├── pdf/
│   └── manual.pdf           # Base PDF template for user manuals
├── db.sql                   # Schema DDL and seed data
├── docker-compose.yml       # PostgreSQL, Redis, MailHog services
├── Makefile                 # Windows build/run/stop targets
└── Makefile_mac_linux       # Unix build/run/stop targets
```

---

## Database Schema

```
users
  id, email, first_name, last_name, password (bcrypt),
  user_active, is_admin, created_at, updated_at

plans
  id, plan_name, plan_amount (cents), created_at, updated_at

user_plans  (join table — one active plan per user)
  id, user_id → users, plan_id → plans, created_at, updated_at
```

**Seed data:**

| Plan | Monthly Price |
|---|---|
| Bronze | $10.00 |
| Silver | $20.00 |
| Gold | $30.00 |

Default admin account: `admin@example.com` (see `db.sql` for bcrypt hash).

---

## Getting Started

### Prerequisites

- Go 1.21+
- Docker Desktop

### 1. Start infrastructure

```bash
docker-compose up -d
```

This starts PostgreSQL (`:5432`), Redis (`:6379`), and MailHog (SMTP `:1025`, web UI `:8025`).

### 2. Load the schema

```bash
# connect to the running postgres container and run the schema
docker exec -i subscription_service-postgres-1 psql -U postgres -d concurrency < db.sql
```

### 3. Run the application

**Windows:**
```bash
make start
```

**macOS / Linux:**
```bash
make start
```

The server starts on `http://localhost:80`.

### 4. View sent emails

Open `http://localhost:8025` in your browser to see all emails captured by MailHog.

---

## API Endpoints

### Public

| Method | Route | Description |
|---|---|---|
| GET | `/` | Home page |
| GET | `/login` | Login form |
| POST | `/login` | Authenticate user |
| GET | `/logout` | Destroy session |
| GET | `/register` | Registration form |
| POST | `/register` | Create account and send activation email |
| GET | `/activate` | Verify signed URL and activate account |

### Protected (`/members/*` — requires active session)

| Method | Route | Description |
|---|---|---|
| GET | `/members/plans` | View all plans with upgrade/downgrade UI |
| GET | `/members/subscribe?id={planID}` | Subscribe to or change a plan |

---

## Concurrency — Design & Significance

Concurrency is the central architectural concern of this project. Every subscription action that involves I/O-bound or CPU-bound work is pushed off the request path into goroutines, keeping HTTP response times fast regardless of how long email delivery or PDF rendering takes.

### 1. Channel-based Email Worker

A dedicated `Mail` struct owns three channels:

```
MailerChan  chan Message   — inbound work queue
ErrorChan   chan error     — surfaces send failures
DoneChan    chan bool      — signals shutdown
```

`listenForMail()` runs as a long-lived background goroutine started at boot. For each message it receives, it spawns an additional goroutine to call `sendMail()`, so multiple emails can be in-flight simultaneously without head-of-line blocking.

```
HTTP handler → sendEmail() → MailerChan → listenForMail() → goroutine → sendMail()
```

This decouples the HTTP layer entirely from SMTP latency.

### 2. Parallel Invoice + PDF Generation on Subscription

When a user subscribes or changes plans, two independent tasks are launched concurrently using `sync.WaitGroup`:

```go
app.Wait.Add(2)

go func() {           // goroutine 1 — invoice email
    defer app.Wait.Done()
    invoice, _ := app.getInvoice(user, plan)
    app.sendEmail(invoiceMsg)
}()

go func() {           // goroutine 2 — PDF manual generation + email
    defer app.Wait.Done()
    pdf := app.generateManual(user, plan)
    pdf.OutputFileAndClose(...)
    app.sendEmail(manualMsg)
}()
```

Both tasks run in parallel. Neither blocks the response — the user is redirected to the plans page immediately while both goroutines complete in the background.

Without concurrency, these two tasks would execute sequentially. Since `generateManual` alone includes a simulated 5-second processing delay (representing real-world PDF rendering), serial execution would add multiple seconds of latency to every plan change. Concurrent execution keeps that cost invisible to the user.

### 3. Graceful Shutdown with WaitGroup Draining

The application listens for `SIGINT`/`SIGTERM` signals. On shutdown, it calls `app.Wait.Wait()` before closing any channels, ensuring that all in-flight invoice and manual goroutines complete before the process exits — preventing partial deliveries or corrupted PDF files.

```
OS Signal → listenForShutdown() → app.Wait.Wait() → signal DoneChan → close all channels → exit
```

### 4. Error Propagation via ErrorChan

Background goroutines cannot return errors to their callers. Instead, errors are sent to `app.ErrorChan`, and a dedicated `listenForErrors()` goroutine logs them. This gives the application full visibility into async failures without requiring synchronous error handling on the hot path.

### Why This Matters

| Concern | Without Concurrency | With Concurrency |
|---|---|---|
| Plan change response time | 5+ seconds (PDF blocks) | Instant redirect |
| Multiple simultaneous emails | Queued serially | Processed in parallel |
| Shutdown data integrity | In-flight work lost | All goroutines drained |
| Async error visibility | Silent failures | Centralized error channel |

---

## Future Scope

### Billing & Payments
- Integrate a payment gateway (Stripe, Razorpay) to charge users on subscription and plan change
- Store payment history and receipts in a `payments` table
- Add prorated billing when upgrading mid-cycle

### Subscription Lifecycle
- Add subscription expiry and auto-renewal with a scheduled background job (cron-style goroutine or a task queue like Asynq)
- Grace period and suspension flow for failed payments
- Free trial support with trial expiry notifications

### User Management
- Password reset via signed email link (the signer infrastructure already exists)
- User profile page to view current plan, billing history, and invoices
- Admin dashboard to view all users and their active subscriptions

### Scalability & Infrastructure
- Replace the in-process `MailerChan` with a proper message broker (RabbitMQ, NATS) so multiple app instances can share a single email worker pool
- Introduce a worker pool with a configurable concurrency limit to prevent goroutine explosion under high load
- Containerize the application and add a `Dockerfile` for deployment
- Add health check endpoints (`/healthz`, `/readyz`) for load balancer integration

### Observability
- Structured logging with `slog` or `zap` instead of the standard `log` package
- Distributed tracing (OpenTelemetry) to track request flow across goroutines
- Prometheus metrics for goroutine counts, email queue depth, and subscription events
- Alerting on `ErrorChan` event rate thresholds

### Security
- CSRF protection on all POST routes
- Rate limiting on login and registration endpoints
- Secure `HttpOnly` flag on session cookies
- Input validation and sanitization on all form fields
- Audit log for sensitive actions (plan changes, logins, failed attempts)

### Testing
- Unit tests for data models with a test database
- Integration tests for the full subscription flow
- Mock SMTP server in tests to assert email content and delivery
- Load tests to validate goroutine behaviour under concurrent plan-change storms
