-- SPDX-License-Identifier: Elastic-2.0

CREATE SCHEMA auth;

CREATE TABLE auth.users (
    id uuid PRIMARY KEY,
    email text NOT NULL,
    name text NOT NULL,
    disabled boolean NOT NULL
);
