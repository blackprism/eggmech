-- migrate:up
create table aca_activity_settings
(
    uuid            varchar(36) primary key,
    guild_id        integer not null,
    channel_id      integer not null,
    minimum_players integer not null,
    minimum_hours   integer not null,
    day_interval    integer not null
);

create index aca_activity_settings_guild_id_channel_id_index
    on aca_activity_settings (guild_id, channel_id);

-- migrate:down
drop table aca_activity_settings;
