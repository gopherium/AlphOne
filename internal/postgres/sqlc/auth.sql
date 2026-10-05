-- SPDX-License-Identifier: Elastic-2.0

CREATE SCHEMA auth;

CREATE TABLE auth.users (
    id uuid PRIMARY KEY,
    disabled boolean NOT NULL
);
