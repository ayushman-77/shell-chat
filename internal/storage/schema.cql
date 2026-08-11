-- Shell Chat Database Schema
-- ScyllaDB / Cassandra Compatible

CREATE KEYSPACE IF NOT EXISTS shell_chat
WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1};

-- Messages table with composite partition key for temporal sharding
-- Partition Key: (channel_id, bucket) distributes load across nodes
-- Clustering Key: message_id DESC for reverse-chronological ordering
CREATE TABLE IF NOT EXISTS shell_chat.messages (
    channel_id bigint,
    bucket int,
    message_id bigint,
    author_id bigint,
    content text,
    created_at timestamp,
    PRIMARY KEY ((channel_id, bucket), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);

-- Users table - primary user profile storage
CREATE TABLE IF NOT EXISTS shell_chat.users (
    user_id bigint PRIMARY KEY,
    username text,
    display_name text,
    password_hash text,
    status int,
    created_at timestamp
);

-- Username lookup table for login
CREATE TABLE IF NOT EXISTS shell_chat.users_by_username (
    username text PRIMARY KEY,
    user_id bigint
);

-- SSH public key to user mapping
CREATE TABLE IF NOT EXISTS shell_chat.user_public_keys (
    fingerprint text PRIMARY KEY,
    user_id bigint,
    public_key_data text
);

-- Guilds (chat servers)
CREATE TABLE IF NOT EXISTS shell_chat.guilds (
    guild_id bigint PRIMARY KEY,
    name text,
    owner_id bigint,
    icon text,
    created_at timestamp
);

-- Guild membership
CREATE TABLE IF NOT EXISTS shell_chat.guild_members (
    guild_id bigint,
    user_id bigint,
    role text,
    joined_at timestamp,
    PRIMARY KEY (guild_id, user_id)
);

-- Channels within guilds
CREATE TABLE IF NOT EXISTS shell_chat.channels (
    guild_id bigint,
    channel_id bigint,
    name text,
    topic text,
    type int,
    position int,
    PRIMARY KEY (guild_id, channel_id)
);

-- Reverse lookup: user -> guilds they belong to
CREATE TABLE IF NOT EXISTS shell_chat.user_guilds (
    user_id bigint,
    guild_id bigint,
    PRIMARY KEY (user_id, guild_id)
);
