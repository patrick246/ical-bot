CREATE TABLE calendars
(
    id             uuid        not null default gen_random_uuid() primary key,
    name           text        not null,
    ical_url       text        not null,
    last_sync_time timestamptz null,
    last_sync_hash bytea       null,
    sync_error_pb  bytea       null
);

CREATE TABLE calendar_default_reminders
(
    id          uuid not null default gen_random_uuid() primary key,
    calendar_id uuid not null references calendars (id) on delete cascade,
    before      interval      default '0 sec'::interval
);

CREATE TABLE channels
(
    id   uuid  not null default gen_random_uuid() primary key,
    type text  not null,
    data jsonb not null
);

CREATE TABLE calendar_channels
(
    calendar_id uuid references calendars (id) on delete cascade,
    channel_id  uuid references channels (id) on delete cascade
);

CREATE TABLE calendar_events
(
    id          uuid  not null default gen_random_uuid() primary key,
    calendar_id uuid  not null references calendars (id) on delete cascade,
    data        bytea not null
);

CREATE TABLE calendar_event_alarms
(
    id uuid not null default gen_random_uuid() primary key,
    event_id uuid not null references calendar_events(id) on delete cascade,
    alarm_time timestamptz not null,
    event_time timestamptz not null
);
