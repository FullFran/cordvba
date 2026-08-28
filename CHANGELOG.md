# Changelog

## [0.1.1](https://github.com/FullFran/eye/compare/v0.1.0...v0.1.1) (2026-08-28)


### Features

* **cli:** add eye daemon, --offline, and persistent answers ([09bdb54](https://github.com/FullFran/eye/commit/09bdb547d2da0bcb51be863cc04a1d91e7e59ba9))
* **cli:** add structured logging with log/slog ([94e5123](https://github.com/FullFran/eye/commit/94e51236594a6215c3b92fdb836637d04a479a6f))
* **cli:** persist source health and report it in eye sources ([e94cbbb](https://github.com/FullFran/eye/commit/e94cbbb15938c9577ccaa13a0c4f5c8757b2d69b))
* **scheduler:** add the polling loop with backoff and a circuit breaker ([13e1840](https://github.com/FullFran/eye/commit/13e184019f6a453c7395199bc281e5aa9917cbf3))
* **store:** add the content-addressed raw cache ([38c3325](https://github.com/FullFran/eye/commit/38c33251b9d0fa8c429e52b95bc2a494b6d13394))
* **store:** add the SQLite record and entity store ([a324d18](https://github.com/FullFran/eye/commit/a324d18bfefaf819928767341f2edd32d8a371eb))


### Documentation

* record the persistence and scheduling slice ([070847e](https://github.com/FullFran/eye/commit/070847e93f8e48bebc256625ac9ada04d5536ae4))

## 0.1.0 (2026-08-28)


### Features

* **cli:** add command registry and version command ([9d90307](https://github.com/FullFran/eye/commit/9d90307c70a217ccc530483eec3686285632a4ca))
* **cli:** add status, news, events, civic, sky, cameras, query and sources ([7d9cb73](https://github.com/FullFran/eye/commit/7d9cb736421720f54a4c8026e9d018cc8d58e899))
* **config:** add environment configuration and the shared HTTP client ([96066bf](https://github.com/FullFran/eye/commit/96066bfd4583c1422cdc5a35222c9c73ac98f4bf))
* **observation:** add Record, Entity, Provenance and geo primitives ([96cbb42](https://github.com/FullFran/eye/commit/96cbb42e7ad55f49818049ff8406480f06986244))
* **observation:** add store ports, an in-memory adapter and the collector ([179e912](https://github.com/FullFran/eye/commit/179e9127d60879410b6f307c89468515bbed30a8))
* **provider:** add adapter ports and source health tracking ([0c3b3f2](https://github.com/FullFran/eye/commit/0c3b3f2abc91f160f46cd0ef45a5bb07f793cec3))
* **provider:** add RSS, CKAN and ADS-B adapters behind a format factory ([a07f177](https://github.com/FullFran/eye/commit/a07f1779abe4a00d0fbb98c468bdeba2e47ec6cb))
* **source:** add source registry domain with a fail-closed automation gate ([56a11fc](https://github.com/FullFran/eye/commit/56a11fcbca7751c098b2f414dab87edadf50be4e))
* **source:** load and validate the registry from YAML ([4ec3190](https://github.com/FullFran/eye/commit/4ec31908cc98ca1db466512adff553097d9b814d))
* **source:** register the Cordoba press and verified feed endpoints ([4808101](https://github.com/FullFran/eye/commit/480810188219c583c3067a51bd8c7a8dd1f77b41))


### Bug Fixes

* **build:** store recorded fixtures byte-for-byte ([86e8f88](https://github.com/FullFran/eye/commit/86e8f888155e4e5fd745365d8b8fb4c04e78cceb))
* **ci:** annotate file reads for gosec, not only golangci-lint ([e1154a0](https://github.com/FullFran/eye/commit/e1154a000203520aa0b6c41917b6c497dd78c697))
* **ci:** use an absolute link in the source-request issue form ([b844997](https://github.com/FullFran/eye/commit/b844997daf0abaa1b911b1f3129e686bafb1b43e))


### Documentation

* add architecture, ADRs, roadmap and data ethics ([8a027b5](https://github.com/FullFran/eye/commit/8a027b51dc837c187e4c7158232a995fc8f3dd43))
* record what v0.1 actually delivers ([0e5dc52](https://github.com/FullFran/eye/commit/0e5dc5202db9abcd24c98f0e528e2fba3eaef276))
