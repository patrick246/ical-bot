alter table calendars
    add column default_reminder_mode text not null default 'DEFAULT_REMINDER_MODE_UNKNOWN';
