# Changelog

## [0.3.2](https://github.com/AtomicWasTaken/surge/compare/v0.3.1...v0.3.2) (2026-03-19)


### Bug Fixes

* make GitHub PR review reruns idempotent ([1c724fd](https://github.com/AtomicWasTaken/surge/commit/1c724fdc46be7d37b529f4d48903e472f46b9002))
* make GitHub PR review reruns idempotent ([b4a55c1](https://github.com/AtomicWasTaken/surge/commit/b4a55c1c3bdc9c414e8d3a65e53843570924d392))

## [0.3.1](https://github.com/AtomicWasTaken/surge/compare/v0.3.0...v0.3.1) (2026-03-18)


### Bug Fixes

* **ci:** upload only archive files in release backfill workflow ([96cf502](https://github.com/AtomicWasTaken/surge/commit/96cf50216220c65bd52ba0f4f8f8ecce85b3f168))


### Reverts

* remove release-please-triggered GoReleaser publish step ([1ff301e](https://github.com/AtomicWasTaken/surge/commit/1ff301e8809d3e72072d9f202a8ce4c5790ea213))

## [0.3.0](https://github.com/AtomicWasTaken/surge/compare/v0.2.0...v0.3.0) (2026-03-18)


### Features

* support codex models via responses endpoint ([376f855](https://github.com/AtomicWasTaken/surge/commit/376f855c9a724578bf3083e73ee3ba2772b5b6a8))


### Bug Fixes

* apply review flag overrides after config load ([96e85f6](https://github.com/AtomicWasTaken/surge/commit/96e85f6f10c1dddcb2188f13aa388d533221d9e6))
* auto-negotiate OpenLimits responses and chat variants ([aa4b03c](https://github.com/AtomicWasTaken/surge/commit/aa4b03c29c873ad1d8b4d339b7336ad60090973a))
* avoid config autodiscovery colliding with built binary ([034c561](https://github.com/AtomicWasTaken/surge/commit/034c56132efcb5a6930dea43ea7a43bcc644dfaf))
* build surge from the checked out source ([3ca326c](https://github.com/AtomicWasTaken/surge/commit/3ca326cccc2be6d7d5f37013ca07742ef53d659a))
* correct go install path to include /cmd/surge ([2e99cbd](https://github.com/AtomicWasTaken/surge/commit/2e99cbd8a91779a1ae9a74f034e5f6c966b9cd7b))
* fallback codex requests to chat completions ([f04abca](https://github.com/AtomicWasTaken/surge/commit/f04abcada0d9c3064356a581f171109303663515))
* make review workflow install and run surge ([c0b4b85](https://github.com/AtomicWasTaken/surge/commit/c0b4b859d40e9bc477b5ff164b4f820d9a90bfba))
* make review workflow install and run surge ([6a7d7ab](https://github.com/AtomicWasTaken/surge/commit/6a7d7abdea920aaa586edc2e32cefb206a16d751))
* normalize base URL and probe more LiteLLM endpoint variants ([6a7e2a1](https://github.com/AtomicWasTaken/surge/commit/6a7e2a114fd4a3f98bc0357e2b47896c385c25ff))
* remove dead code in vibe.go that caused lint error ([b1e63ff](https://github.com/AtomicWasTaken/surge/commit/b1e63ff3d2c715dd3eb95053d388e49c02887326))
* remove push-to-main trigger from review workflow and add goreleaser release workflow ([be703b7](https://github.com/AtomicWasTaken/surge/commit/be703b772e8b796faf78524efa685ec01d16e2f6))
* replace non-existent install URL with go install ([2941eec](https://github.com/AtomicWasTaken/surge/commit/2941eecdfd8020e5e9d29927707b06b9b3a6dd2a))
* retry OpenLimits responses without token limit field ([12ecfb6](https://github.com/AtomicWasTaken/surge/commit/12ecfb6f164318391aba00d51d05f426352572bf))
* retry responses requests without temperature ([eb4c892](https://github.com/AtomicWasTaken/surge/commit/eb4c892b2dbde19bad146215f552a6a17f11e542))
* send responses input as list for codex models ([96187b0](https://github.com/AtomicWasTaken/surge/commit/96187b0677e38909a4c464e155b3ab86a341358f))
* update goreleaser config to v2 format (remove projectName) ([d8bf7d1](https://github.com/AtomicWasTaken/surge/commit/d8bf7d1259b5e310372853f1a2595fd556868486))
* use a LiteLLM-compatible default model ([95b8dbd](https://github.com/AtomicWasTaken/surge/commit/95b8dbd05253aa1e7ed8539cf742c8256442eb53))

## [0.2.0](https://github.com/AtomicWasTaken/surge/compare/surge-v0.1.0...surge-v0.2.0) (2026-03-18)


### Features

* support codex models via responses endpoint ([376f855](https://github.com/AtomicWasTaken/surge/commit/376f855c9a724578bf3083e73ee3ba2772b5b6a8))


### Bug Fixes

* apply review flag overrides after config load ([96e85f6](https://github.com/AtomicWasTaken/surge/commit/96e85f6f10c1dddcb2188f13aa388d533221d9e6))
* auto-negotiate OpenLimits responses and chat variants ([aa4b03c](https://github.com/AtomicWasTaken/surge/commit/aa4b03c29c873ad1d8b4d339b7336ad60090973a))
* avoid config autodiscovery colliding with built binary ([034c561](https://github.com/AtomicWasTaken/surge/commit/034c56132efcb5a6930dea43ea7a43bcc644dfaf))
* build surge from the checked out source ([3ca326c](https://github.com/AtomicWasTaken/surge/commit/3ca326cccc2be6d7d5f37013ca07742ef53d659a))
* correct go install path to include /cmd/surge ([2e99cbd](https://github.com/AtomicWasTaken/surge/commit/2e99cbd8a91779a1ae9a74f034e5f6c966b9cd7b))
* fallback codex requests to chat completions ([f04abca](https://github.com/AtomicWasTaken/surge/commit/f04abcada0d9c3064356a581f171109303663515))
* make review workflow install and run surge ([c0b4b85](https://github.com/AtomicWasTaken/surge/commit/c0b4b859d40e9bc477b5ff164b4f820d9a90bfba))
* make review workflow install and run surge ([6a7d7ab](https://github.com/AtomicWasTaken/surge/commit/6a7d7abdea920aaa586edc2e32cefb206a16d751))
* normalize base URL and probe more LiteLLM endpoint variants ([6a7e2a1](https://github.com/AtomicWasTaken/surge/commit/6a7e2a114fd4a3f98bc0357e2b47896c385c25ff))
* remove dead code in vibe.go that caused lint error ([b1e63ff](https://github.com/AtomicWasTaken/surge/commit/b1e63ff3d2c715dd3eb95053d388e49c02887326))
* remove push-to-main trigger from review workflow and add goreleaser release workflow ([be703b7](https://github.com/AtomicWasTaken/surge/commit/be703b772e8b796faf78524efa685ec01d16e2f6))
* replace non-existent install URL with go install ([2941eec](https://github.com/AtomicWasTaken/surge/commit/2941eecdfd8020e5e9d29927707b06b9b3a6dd2a))
* retry OpenLimits responses without token limit field ([12ecfb6](https://github.com/AtomicWasTaken/surge/commit/12ecfb6f164318391aba00d51d05f426352572bf))
* retry responses requests without temperature ([eb4c892](https://github.com/AtomicWasTaken/surge/commit/eb4c892b2dbde19bad146215f552a6a17f11e542))
* send responses input as list for codex models ([96187b0](https://github.com/AtomicWasTaken/surge/commit/96187b0677e38909a4c464e155b3ab86a341358f))
* update goreleaser config to v2 format (remove projectName) ([d8bf7d1](https://github.com/AtomicWasTaken/surge/commit/d8bf7d1259b5e310372853f1a2595fd556868486))
* use a LiteLLM-compatible default model ([95b8dbd](https://github.com/AtomicWasTaken/surge/commit/95b8dbd05253aa1e7ed8539cf742c8256442eb53))

## Changelog

All notable changes to this project will be documented in this file.
