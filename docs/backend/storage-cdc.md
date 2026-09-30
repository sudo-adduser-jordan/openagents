# Storage and CDC

Durable facts, SQLite schema, and the change-data-capture pipeline. Canonical contract: [overview.md](../overview.md).

## Persistence and CDC

### SQLite Schema

```mermaid
erDiagram
    projects o|--o{ sessions : optionally_owns
    projects o|--o| conversations : optionally_owns_manager_narrative
    sessions ||--o| conversations : owns_worker_narrative
    sessions ||--o{ session_interface_transitions : records_controller_handoffs
    session_interface_transitions ||--o{ session_interface_transition_messages : holds_messages_during_gap
    conversations ||--o{ conversation_turns : contains
    conversations ||--o{ conversation_messages : contains
    conversations ||--o{ conversation_activities : contains
    sessions ||--o{ pull_requests : owns
    pull_requests ||--o{ pr_checks : has
    pull_requests ||--o{ pr_review_threads : has
    pull_requests ||--o{ pr_comments : has
    sessions ||--o{ notifications : has
    change_log }o--o| projects : optionally_tracks
    change_log }o--o| sessions : optionally_tracks
    change_log }|--|| pull_requests : tracks

    projects {
        string id PK
        string name
        string repo
        jsonb config
    }

    sessions {
        string id PK
        string project_id FK "nullable for standalone workers"
        string harness
        string session_mode
        string runtime_handle_id
        string provider_conversation_id
        string controller_generation
        string activity_state
        boolean is_terminated
        jsonb metadata
    }

    conversations {
        string id PK
        string scope
        string project_id FK "nullable for standalone conversations"
        string session_id FK
        string current_session_id FK
        integer latest_sequence
    }

    pull_requests {
        string id PK
        string session_id FK
        integer number
        string state
        string title
        boolean draft
        boolean mergeable
    }

    pr_checks {
        string id PK
        string pr_id FK
        string name
        string status
        string conclusion
    }

    change_log {
        bigint seq PK
        string table_name
        string row_id
        string operation
        jsonb old_data
        jsonb new_data
    }
```

### CDC Pipeline

```mermaid
flowchart LR
    DB[(SQLite)] -->|INSERT/UPDATE/DELETE| Trigger[DB Trigger]
    Trigger -->|append| ChangeLog[change_log]
    ChangeLog -->|poll| Poller[CDC Poller]
    Poller -->|decode| Decoder[Event Decoder]
    Decoder -->|Event| Broadcaster[Broadcaster]
    Broadcaster -->|callback| Sub1[Terminal Fanout]
    Broadcaster -->|callback| Sub2[SSE Writer]
    Broadcaster -->|callback| Sub3[Cache Invalidation]

    Poller -->|watermark| Watermark[seq tracking]
    Watermark -->|resume position| Poller

```

---

## Sources of truth

- Migrations and queries: `backend/internal/storage/sqlite/migrations/`, `backend/internal/storage/sqlite/queries/`
- Generated code: `backend/internal/storage/sqlite/gen/` (via `task db:sqlc`; never hand-edit)
