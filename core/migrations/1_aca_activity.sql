-- migrate:up
create table aca_activity
(
    uuid          varchar(36)       not null primary key,
    guild_id      integer           not null,
    user_id       integer           not null,
    activity_name varchar(256),
    started_at    integer           not null,
    duration      integer default 0 not null
);

-- migrate:down
drop table aca_activity;
