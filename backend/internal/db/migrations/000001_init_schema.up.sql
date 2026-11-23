CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- 1. Users
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL,
    name TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    global_role TEXT NOT NULL DEFAULT 'USER' CHECK (global_role IN ('SUPER_ADMIN', 'USER')),
    is_active BOOLEAN NOT NULL DEFAULT true,
    must_change_password BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_users_lower_email ON users (lower(email));

-- 2. Workspaces
CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL CHECK (char_length(name) >= 2 AND char_length(name) <= 100),
    slug TEXT NOT NULL UNIQUE CHECK (char_length(slug) >= 3 AND char_length(slug) <= 50),
    description TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 500),
    created_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    archived_at TIMESTAMPTZ
);

-- 3. Workspace Members
CREATE TABLE workspace_members (
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('ADMIN', 'MEMBER', 'VIEWER')),
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    added_by UUID REFERENCES users(id),
    PRIMARY KEY (workspace_id, user_id)
);

-- 4. Work Items (Task/Bug)
CREATE TABLE work_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    type TEXT NOT NULL CHECK (type IN ('TASK', 'BUG')),
    title TEXT NOT NULL CHECK (char_length(trim(title)) >= 3 AND char_length(title) <= 180),
    description TEXT NOT NULL DEFAULT '' CHECK (char_length(description) <= 20000),
    status TEXT NOT NULL DEFAULT 'TODO' CHECK (status IN ('TODO', 'IN_PROGRESS', 'BLOCKED', 'IN_REVIEW', 'DONE')),
    priority TEXT NOT NULL DEFAULT 'MEDIUM' CHECK (priority IN ('LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    assignee_id UUID REFERENCES users(id) ON DELETE SET NULL,
    created_by UUID NOT NULL REFERENCES users(id),
    updated_by UUID NOT NULL REFERENCES users(id),
    due_date DATE,
    steps_to_reproduce TEXT,
    expected_result TEXT,
    actual_result TEXT,
    version INT NOT NULL DEFAULT 1 CHECK (version >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT uq_work_items_workspace_id_id UNIQUE (workspace_id, id)
);

-- 5. Item Attachments (dengan Composite Foreign Key ke work_items)
CREATE TABLE item_attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    item_id UUID NOT NULL,
    uploader_id UUID NOT NULL REFERENCES users(id),
    original_filename TEXT NOT NULL,
    storage_key TEXT NOT NULL UNIQUE,
    mime_type TEXT NOT NULL,
    byte_size BIGINT NOT NULL CHECK (byte_size > 0),
    width INT CHECK (width >= 0),
    height INT CHECK (height >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_item_attachments_item FOREIGN KEY (workspace_id, item_id) REFERENCES work_items(workspace_id, id) ON DELETE CASCADE
);

-- 6. Sessions
CREATE TABLE sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    verifier_hash BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    idle_expires_at TIMESTAMPTZ NOT NULL,
    absolute_expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);

-- 7. Audit Events
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    workspace_id UUID REFERENCES workspaces(id) ON DELETE SET NULL,
    entity_type TEXT NOT NULL,
    entity_id UUID,
    action TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Performance Indexes (SSOT §6)
CREATE INDEX idx_work_items_workspace_list ON work_items (workspace_id, deleted_at, updated_at DESC, id DESC);
CREATE INDEX idx_work_items_workspace_status ON work_items (workspace_id, status, deleted_at);
CREATE INDEX idx_work_items_workspace_priority ON work_items (workspace_id, priority, deleted_at);
CREATE INDEX idx_work_items_workspace_assignee ON work_items (workspace_id, assignee_id, deleted_at);
CREATE INDEX idx_work_items_workspace_type ON work_items (workspace_id, type, deleted_at);
CREATE INDEX idx_work_items_workspace_due_date ON work_items (workspace_id, due_date, deleted_at);

CREATE INDEX idx_item_attachments_workspace_item ON item_attachments (workspace_id, item_id);
CREATE INDEX idx_audit_events_workspace_entity ON audit_events (workspace_id, entity_type, entity_id, created_at DESC);
CREATE INDEX idx_sessions_user_revoked ON sessions (user_id, revoked_at);
