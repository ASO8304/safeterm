# ADR-038: Resource-only packaged log registration

Status: Accepted

## Context

The packaged mapped log operation and resources.d logs both exposed log menus.
This gave administrators two overlapping places to configure application logs
and displayed sample sources without an administrator resource definition.

## Decision

Remove the packaged log operation and its policy grants. Keep log-viewer as an
empty default role that resources.d definitions can grant, and keep the isolated
log-reader profile and tail executable required for generated log operations.
Do not create the sample service_A.log during installation. Register application
logs through resources.d with exact paths and explicit roles; compile generated
operations as before. No arbitrary path input or expanded filesystem permission
is introduced. Generic administrator-defined operations remain supported.

## Upgrade behavior

Rebuild candidates from new packaged defaults and preserved site inputs. Never
delete administrator operations.d overrides or actual log files automatically.
Explicit old log overrides may reintroduce the old menu until the administrator
migrates their paths and removes/disables those overrides. No migration of
credentials or ownership is needed.

## Verification

Regression tests reject packaged log operations/policy grants and sample-file
creation. Terminal tests check that only registered resource names appear in
log completion and retain authorization and signed operation-ID checks. Generic
mapped-argument isolation tests use explicit test-only fixtures.
