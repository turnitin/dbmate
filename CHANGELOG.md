## 1.6.0

* Add explicit lock timeout (currently 30 seconds) - https://github.com/turnitin/dbmate/pull/9
* Update mysql driver to 1.5.1 - https://github.com/turnitin/dbmate/pull/10

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
