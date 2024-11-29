-- migrate:up
create table aca_activity_settings
(
    uuid            varchar(36) primary key,
    guild_id        integer not null,
    activity_name   varchar(256),
    minimum_players integer not null,
    minimum_hours   integer not null,
    day_interval    integer not null
);

-- migrate:down
drop table aca_activity_settings;
