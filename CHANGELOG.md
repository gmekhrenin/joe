# Changelog

## [Unreleased]

## [0.4.0] - 2020-02-06

- Dockerize the Joe application. Main images are stored in Docker Hub, devel versions are in GitLab Registry. **Database Lab is now a requirement**.
- Provisioning code fully replaced by Database Lab Client SDK.
- Use new synchronous methods from Database Lab SDK.
- Use a single Postgres connection per user session. It helps to use `set`, `reset`.
- Support idle sessions on the Joe side: if Postgres session is closed, when a new user request arrives, Joe automatically opens a new Postgres connection.
- Migrate to Go modules.
- Refactor the psql runner.
- Various updates to README, documentation.
- Print Joe version (now `0.4.0`) when starting a session.
- Print `dataStateAt` value when starting a session – necessary to understand the lag of the snapshot compared to the origin.