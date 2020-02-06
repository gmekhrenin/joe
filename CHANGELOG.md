# Changelog

## [Unreleased]

## [0.4.0] - 2020-02-06

- Use Client SDK instead of provision code which duplicates Database Lab server code.
- Support idle sessions on the Joe side.
- Use new synchronous methods from Database Lab SDK.
- Use a single connection per user session.
- Remove the AWS part and other modes completely, use SDK instead.
- Dockerize the Joe application.
- Build and push Docker images to Docker Hub and Gitlab Registry.
- Migrate to Go modules.
- Refactor a psql runner.
- Update README.

