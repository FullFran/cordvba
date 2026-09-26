# Changelog

## [0.4.0](https://github.com/FullFran/cordvba/compare/v0.3.0...v0.4.0) (2026-09-26)


### Features

* **scheduler:** compose deployment overrides into scheduling and retention ([#135](https://github.com/FullFran/cordvba/issues/135)) ([7881f1f](https://github.com/FullFran/cordvba/commit/7881f1f84559c2a1d84e524f69d009902d3e765d))
* **source:** parse and validate optional deployment overrides ([#134](https://github.com/FullFran/cordvba/issues/134)) ([1083765](https://github.com/FullFran/cordvba/commit/108376502251ee82fe3906bac01103cfb48d75dc))
* **source:** surface operator-disabled sources and document overrides ([#136](https://github.com/FullFran/cordvba/issues/136)) ([43217d6](https://github.com/FullFran/cordvba/commit/43217d65fd9957024c367be651eb470e1186ce34)), closes [#125](https://github.com/FullFran/cordvba/issues/125)


### Bug Fixes

* **cli:** apply source retention on the daemon path ([#133](https://github.com/FullFran/cordvba/issues/133)) ([46b34f6](https://github.com/FullFran/cordvba/commit/46b34f68f718b3cffb13d4c602ba991e53e42a30)), closes [#132](https://github.com/FullFran/cordvba/issues/132)
* **provider:** filter miteco-ica by province code instead of a bounding box ([#107](https://github.com/FullFran/cordvba/issues/107)) ([e2459ac](https://github.com/FullFran/cordvba/commit/e2459acc72bc3d344fa9a02f7c5ad26d1afcccef)), closes [#98](https://github.com/FullFran/cordvba/issues/98)
* **provider:** read aemet observation timestamps with a numeric offset ([#144](https://github.com/FullFran/cordvba/issues/144)) ([13ba9da](https://github.com/FullFran/cordvba/commit/13ba9daec095462ebb28d3457f7194b2bb49867b)), closes [#143](https://github.com/FullFran/cordvba/issues/143)
* **provider:** scope aemet-warnings to Cordoba province ([#145](https://github.com/FullFran/cordvba/issues/145)) ([29a80f5](https://github.com/FullFran/cordvba/commit/29a80f50efab82f300ed4875ce15736e03df4935)), closes [#142](https://github.com/FullFran/cordvba/issues/142)
* **source:** keep environmental series as history instead of expiring them ([#126](https://github.com/FullFran/cordvba/issues/126)) ([ec9d37b](https://github.com/FullFran/cordvba/commit/ec9d37bf351ee4fff87f3999f8ff2b2177a4ad4f)), closes [#73](https://github.com/FullFran/cordvba/issues/73)
* **store:** prune raw payloads no surviving record references ([#127](https://github.com/FullFran/cordvba/issues/127)) ([d3c2652](https://github.com/FullFran/cordvba/commit/d3c265201d4ad8d4984d4dcda63425d9d59fc421)), closes [#74](https://github.com/FullFran/cordvba/issues/74)


### Refactoring

* **build:** move eye into apps/eye of the cordvba monorepo ([#97](https://github.com/FullFran/cordvba/issues/97)) ([c4dbae1](https://github.com/FullFran/cordvba/commit/c4dbae12a1e4d028863c4f45cd40a5e79891743b))

## [0.3.0](https://github.com/FullFran/eye/compare/v0.2.0...v0.3.0) (2026-08-28)


### Features

* **cli:** browse and choose a camera interactively ([116d8d7](https://github.com/FullFran/eye/commit/116d8d7a1e5260cb7977779c633fea496f45718e))
* **cli:** show camera frames with the age of the image ([c0a9758](https://github.com/FullFran/eye/commit/c0a97580d6ad6e76410a3bcec94a79965c90ab42))
* **source:** connect IGN seismicity, IMAE and public procurement ([3d1c532](https://github.com/FullFran/eye/commit/3d1c532e9bfa96a5eab1d0cac9eb73db37f1074e))


### Documentation

* correct the camera findings and record why there is no stream ([fa9d099](https://github.com/FullFran/eye/commit/fa9d09934a6c79058232184fd743ea38e8788628))
* **source:** record that Cordoba publishes no camera image ([d7015f1](https://github.com/FullFran/eye/commit/d7015f11bd03e1fa8db12929102e33bff3cd2b1d))

## [0.2.0](https://github.com/FullFran/eye/compare/v0.1.0...v0.2.0) (2026-08-28)


### Features

* **cli:** add eye daemon, --offline, and persistent answers ([09bdb54](https://github.com/FullFran/eye/commit/09bdb547d2da0bcb51be863cc04a1d91e7e59ba9))
* **cli:** add structured logging with log/slog ([94e5123](https://github.com/FullFran/eye/commit/94e51236594a6215c3b92fdb836637d04a479a6f))
* **cli:** persist source health and report it in eye sources ([e94cbbb](https://github.com/FullFran/eye/commit/e94cbbb15938c9577ccaa13a0c4f5c8757b2d69b))
* **scheduler:** add the polling loop with backoff and a circuit breaker ([13e1840](https://github.com/FullFran/eye/commit/13e184019f6a453c7395199bc281e5aa9917cbf3))
* **store:** add the content-addressed raw cache ([38c3325](https://github.com/FullFran/eye/commit/38c33251b9d0fa8c429e52b95bc2a494b6d13394))
* **store:** add the SQLite record and entity store ([a324d18](https://github.com/FullFran/eye/commit/a324d18bfefaf819928767341f2edd32d8a371eb))


### Bug Fixes

* **build:** pin a patched Go toolchain ([c51921b](https://github.com/FullFran/eye/commit/c51921b0aaf897ca4d262f55d7551a2c65f74c25))


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
