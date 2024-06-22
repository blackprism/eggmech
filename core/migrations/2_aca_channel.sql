-- migrate:up
create table aca_channel
(
    uuid        varchar(36) not null,
    channel_id  integer     not null,
    name        varchar(18) not null,
    enabled_at  integer     not null,
    archived_at integer
);

-- migrate:down
drop table aca_channel;
