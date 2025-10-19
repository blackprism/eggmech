-- migrate:up
create table aca_activity_channel
(
    uuid                    varchar(36)     primary key,
    activity_settings_uuid  varchar(36)     not null,
    activity_name           varchar(256)    not null,

    constraint aca_activity_channel_aca_activity_settings_fk
            foreign key (activity_settings_uuid) references aca_activity_settings (uuid)
);

-- migrate:down
drop table aca_activity_settings;
