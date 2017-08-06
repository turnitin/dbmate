## 1.5.0

* Add 'force' command to record the unapplied migrations from the filesystem in
  the migrations table, but without actually applying them.

## 1.4.0

* Enable Postgres migrations to acquire an advisory lock to ensure they will
  execute a single migration run at a time.

## 1.3.0

* Add support for projects
* Add darwin as a build target

## 1.2.1

* Initial fork from amacneil/dbmate
