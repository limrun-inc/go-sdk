# Changelog

## 0.10.0 (2026-08-04)

Full Changelog: [v0.9.0...v0.10.0](https://github.com/limrun-inc/go-sdk/compare/v0.9.0...v0.10.0)

### Features

* **api:** add analytics api ([ce7c027](https://github.com/limrun-inc/go-sdk/commit/ce7c027b68f549af13b25e15ec9505370c03d849))
* **api:** add apiUrl to android ([becfdd5](https://github.com/limrun-inc/go-sdk/commit/becfdd5cc2d69ff4cec1e2c787ebde732021657f))
* **api:** add gradle to stainless config as well ([19541d3](https://github.com/limrun-inc/go-sdk/commit/19541d36c9b8b60f064c83f315235f0e3e06e041))
* **api:** add jurisdiction ([f44c4ad](https://github.com/limrun-inc/go-sdk/commit/f44c4ad0868975448d7461daeeda9d599c146fbc))
* **api:** add scoped tokens to stainless config ([2ce028c](https://github.com/limrun-inc/go-sdk/commit/2ce028c6c59fb53e926fe5e129bbd5153a872057))
* **api:** add spec.model to ios creation api ([c876fe5](https://github.com/limrun-inc/go-sdk/commit/c876fe5abe47fb1c3c75489596b504a597398ea9))
* **api:** add status.mcpUrl to android ([a8f1902](https://github.com/limrun-inc/go-sdk/commit/a8f1902294a19df4d3507d96eec9f16e591dda13))
* **api:** add version to android playwright ([bf95382](https://github.com/limrun-inc/go-sdk/commit/bf95382a074080867f35b874fd8ae03688017b66))
* **api:** add xcode build logs and signedStreamUrl ([417e67c](https://github.com/limrun-inc/go-sdk/commit/417e67cb41a9d6e743c3a454c8b3808f6c89d923))
* **api:** add xcode instances endpoints ([87ad94c](https://github.com/limrun-inc/go-sdk/commit/87ad94cec4f19318f46206b81b4003028f4ccc28))
* **api:** analytics api, make platform enum ([8c6a2da](https://github.com/limrun-inc/go-sdk/commit/8c6a2dafa7a2a27120709217978a96579b22d265))
* **api:** fix indentation ([24bc445](https://github.com/limrun-inc/go-sdk/commit/24bc445d26edbffaf1a15c3e992529822899d75a))
* **api:** fix model name for xcode_instance in stainless config ([e43f5f7](https://github.com/limrun-inc/go-sdk/commit/e43f5f71a357e213a299c9b5098bab995846eea1))
* **api:** fix stainless models for scoped tokens ([369f31e](https://github.com/limrun-inc/go-sdk/commit/369f31e98538293c6dcc191474d1592b05e7ef7f))
* **api:** ios add forceBundleId ([afa244e](https://github.com/limrun-inc/go-sdk/commit/afa244e214670b06f40a679251ea8306258d039a))
* **api:** manual updates ([dacfda4](https://github.com/limrun-inc/go-sdk/commit/dacfda4c9a41eb762c2eec96938e7a3a2e6dc198))
* **api:** manual updates ([9b79c70](https://github.com/limrun-inc/go-sdk/commit/9b79c705ce79c86013bc60a526f90ccdf1c39236))
* **api:** manual updates ([79c6b51](https://github.com/limrun-inc/go-sdk/commit/79c6b51dee21d68ed2447fac72f566fcc477a6de))
* **api:** scoped tokens desc fix ([ab5eb1c](https://github.com/limrun-inc/go-sdk/commit/ab5eb1c32f2f47bf982bdfea5f7b920dc7dc12fd))
* **api:** simplify the models for analytics endpoint ([a45fddf](https://github.com/limrun-inc/go-sdk/commit/a45fddf1c007f4bd867c3ff57bc981be81a9ed9f))
* **api:** update stainless config for xcode_instances resource ([420adf1](https://github.com/limrun-inc/go-sdk/commit/420adf130fa27758c61fb1843af45023b2598ba3))
* **api:** update to latest main ([b29a9c7](https://github.com/limrun-inc/go-sdk/commit/b29a9c7125134620f7e6efdabf573293e3ecca42))
* **internal:** support comma format in multipart form encoding ([167880a](https://github.com/limrun-inc/go-sdk/commit/167880a0ca4eebbab7ec0ac7447283aaab8cde1f))
* **stlc:** configurable CI runner and private-production-repo support in workflow templates ([179bdfc](https://github.com/limrun-inc/go-sdk/commit/179bdfc90aace0b72cfc40e0b0ae4df10cb74ef9))


### Bug Fixes

* allow canceling a request while it is waiting to retry ([2b24378](https://github.com/limrun-inc/go-sdk/commit/2b243788f6493d2a567ecb82d88ec7eb21034416))
* **internal:** skip tests that depend on mock server ([08703b0](https://github.com/limrun-inc/go-sdk/commit/08703b0861f20334a8e7642e09ebea85fa196744))
* prevent duplicate ? in query params ([3ba98cd](https://github.com/limrun-inc/go-sdk/commit/3ba98cdca12fd52de92b30a78a4f016b36aee6bd))


### Chores

* **ci:** skip lint on metadata-only changes ([558c7fc](https://github.com/limrun-inc/go-sdk/commit/558c7fcd716046017118e560720aff547a6ec6cc))
* **ci:** skip uploading artifacts on stainless-internal branches ([5b9bc78](https://github.com/limrun-inc/go-sdk/commit/5b9bc781fd6c18a5521cc4a79288269f5d4e4516))
* **ci:** support opting out of skipping builds on metadata-only commits ([9f914bd](https://github.com/limrun-inc/go-sdk/commit/9f914bd9ba290e05877165638874484712de428f))
* **client:** fix multipart serialisation of Default() fields ([759d9ea](https://github.com/limrun-inc/go-sdk/commit/759d9eaa5ba52007d9fcb53d649e9828f547a4d7))
* **internal:** codegen related update ([44b0352](https://github.com/limrun-inc/go-sdk/commit/44b035295cb62cc133e4b2b9fa6f5daa85165d43))
* **internal:** codegen related update ([c0a4ba5](https://github.com/limrun-inc/go-sdk/commit/c0a4ba5fc6e9b5518070a5391306e6485bade2db))
* **internal:** codegen related update ([6698aee](https://github.com/limrun-inc/go-sdk/commit/6698aee88afa08ff6a0ba2bf2c21216647d5a96c))
* **internal:** codegen related update ([8adc4fa](https://github.com/limrun-inc/go-sdk/commit/8adc4fa9d34bad3b8c883654557a2f4aa981ec6b))
* **internal:** codegen related update ([47a92fe](https://github.com/limrun-inc/go-sdk/commit/47a92fe34b11b61efa5abe39f6e1e9882f379687))
* **internal:** codegen related update ([660aded](https://github.com/limrun-inc/go-sdk/commit/660aded4427c99471caa3f869cc14c5abc19a2d6))
* **internal:** codegen related update ([5382d75](https://github.com/limrun-inc/go-sdk/commit/5382d750467ece09844a71398ddeca322b37c801))
* **internal:** minor cleanup ([e413e98](https://github.com/limrun-inc/go-sdk/commit/e413e985c64ad6e3eb2b52852696d518607f1574))
* **internal:** move custom custom `json` tags to `api` ([9f208bc](https://github.com/limrun-inc/go-sdk/commit/9f208bc522bb98b07ccd8300da97c147bae8b0eb))
* **internal:** remove mock server code ([823a8f0](https://github.com/limrun-inc/go-sdk/commit/823a8f03a6bb5a323b65cd284f250642cbdbc335))
* **internal:** support default value struct tag ([78cbe02](https://github.com/limrun-inc/go-sdk/commit/78cbe021375f74fc4fbd790011c725fca995a6df))
* **internal:** tweak CI branches ([39643cc](https://github.com/limrun-inc/go-sdk/commit/39643cc242eb93ad0b06b12ff4ad436236b11413))
* **internal:** update gitignore ([e99a4bc](https://github.com/limrun-inc/go-sdk/commit/e99a4bc083089ed12ce6711d14a9da1d5e6a1b21))
* **internal:** use explicit returns ([ba81464](https://github.com/limrun-inc/go-sdk/commit/ba814646255196feecc63384749a72817f906912))
* **internal:** use explicit returns in more places ([525abc0](https://github.com/limrun-inc/go-sdk/commit/525abc0e1a4c7b3fd105b5acdcb13608c4d1fec9))
* remove unnecessary error check for url parsing ([b9f32c7](https://github.com/limrun-inc/go-sdk/commit/b9f32c749541443e6a52bffc9590805583acdd9a))
* update docs for api:"required" ([4d52f9c](https://github.com/limrun-inc/go-sdk/commit/4d52f9cb2d7c2bc366027705ce4f2f5e5505849d))
* update mock server docs ([dab2133](https://github.com/limrun-inc/go-sdk/commit/dab213314cf00b7b1eccf53b34bdfe5f739b3a93))

## 0.9.0 (2026-02-11)

Full Changelog: [v0.8.0...v0.9.0](https://github.com/limrun-inc/go-sdk/compare/v0.8.0...v0.9.0)

### Features

* **api:** add displayName to asset ([0e158a0](https://github.com/limrun-inc/go-sdk/commit/0e158a0c6d8000bd2e44a77f57c81e4163a81acb))
* **api:** add ios sandbox properties and app store for assets ([1c50018](https://github.com/limrun-inc/go-sdk/commit/1c50018ee90ac3922b70d3b3232ce218f55b6504))
* **api:** add optional os field to assets ([eb6f431](https://github.com/limrun-inc/go-sdk/commit/eb6f431684382700dc16b5e2093f12929aadb110))
* **api:** add status.mcpUrl for ios ([f606eec](https://github.com/limrun-inc/go-sdk/commit/f606eec9377b1f834b447663c5a8cb0726e24f27))
* **client:** add a convenient param.SetJSON helper ([da032fb](https://github.com/limrun-inc/go-sdk/commit/da032fbc4cd28b32dfb98acdd5316ed8e863b39b))


### Bug Fixes

* **docs:** add missing pointer prefix to api.md return types ([f1f6ed6](https://github.com/limrun-inc/go-sdk/commit/f1f6ed6744f3ccd6ca25be3cc56e9e5725e3b95d))
* **encoder:** correctly serialize NullStruct ([80cb5d8](https://github.com/limrun-inc/go-sdk/commit/80cb5d87f4c1945654e0e648e98a6a635f76a731))
* skip usage tests that don't work with Prism ([bfca4af](https://github.com/limrun-inc/go-sdk/commit/bfca4af9a3431b9cfa12c290b4dd3634dccb3e6a))


### Chores

* add float64 to valid types for RegisterFieldValidator ([04f66e5](https://github.com/limrun-inc/go-sdk/commit/04f66e5a22b0ff782201bc5b0555e792a09ff3d0))
* **internal:** update `actions/checkout` version ([604a7bc](https://github.com/limrun-inc/go-sdk/commit/604a7bc474b13c1058941c17bb20f43206e2cf2d))


### Documentation

* add more examples ([32ab661](https://github.com/limrun-inc/go-sdk/commit/32ab661cfa367a8d6a313d8fbde232ba3c7b3853))

## 0.8.0 (2025-12-15)

Full Changelog: [v0.7.0...v0.8.0](https://github.com/limrun-inc/go-sdk/compare/v0.7.0...v0.8.0)

### Features

* **api:** add android sandbox api ([1ebbc62](https://github.com/limrun-inc/go-sdk/commit/1ebbc6211465635a7e06d2caf45179cd511fd4a0))
* **api:** add asset type configuration with chrome flag ([f0259ce](https://github.com/limrun-inc/go-sdk/commit/f0259ce84f1d4146ba8c86851924895cd56e69e9))
* **api:** add the optional errorMessage field in status ([75f6be0](https://github.com/limrun-inc/go-sdk/commit/75f6be0d675d917bd28b909d724222e616cfc216))
* **api:** make chromeFlag enum with supported value ([4bd9879](https://github.com/limrun-inc/go-sdk/commit/4bd9879a8b02f431fed31bcc00460116efcec01d))
* **api:** manual updates ([4d9e4be](https://github.com/limrun-inc/go-sdk/commit/4d9e4be4bfd16aaf4f6da39fa8ad3c4801602f30))
* **api:** manual updates ([ca417f1](https://github.com/limrun-inc/go-sdk/commit/ca417f1298add1f830e5c982d8a71d64c0a99c1b))
* **encoder:** support bracket encoding form-data object members ([5964857](https://github.com/limrun-inc/go-sdk/commit/596485735b8895a73364bb2ed779da9fb4d54a16))


### Bug Fixes

* **client:** correctly specify Accept header with */* instead of empty ([42d4d55](https://github.com/limrun-inc/go-sdk/commit/42d4d5577655d9f2f96b4cc590ded1b3ea698244))
* **mcp:** correct code tool API endpoint ([64ecd45](https://github.com/limrun-inc/go-sdk/commit/64ecd45b9797ae6d15a32f4864fe37bb37aed4ec))
* rename param to avoid collision ([8023146](https://github.com/limrun-inc/go-sdk/commit/8023146858cfc83bd8eb435b93931b82c40522a3))


### Chores

* bump gjson version ([978c3d9](https://github.com/limrun-inc/go-sdk/commit/978c3d93de6cb43297a70bf2164ff5d9313e64ad))
* elide duplicate aliases ([bbaa5e7](https://github.com/limrun-inc/go-sdk/commit/bbaa5e739978a9b2b93642fd96412e21bed69c34))
* **internal:** codegen related update ([d92de46](https://github.com/limrun-inc/go-sdk/commit/d92de46bdfe93470baee5c04109f88f6c3997779))

## 0.7.0 (2025-11-11)

Full Changelog: [v0.6.0...v0.7.0](https://github.com/limrun-inc/go-sdk/compare/v0.6.0...v0.7.0)

### Features

* **api:** add assetId as asset source kind ([56c0220](https://github.com/limrun-inc/go-sdk/commit/56c0220d990c5c6a3d0704b0fa3b07e18ea43c56))
* **api:** add reuseIfExists to creation endpoint ([dfb5f86](https://github.com/limrun-inc/go-sdk/commit/dfb5f865f4fd1477cfddb922d65a6e7e9b0f50b0))
* **api:** update to use LIM_API_KEY instead of LIM_TOKEN ([0e5edaf](https://github.com/limrun-inc/go-sdk/commit/0e5edaf3346ce21c452337d8ff50d4c77787d7e9))

## 0.6.0 (2025-11-07)

Full Changelog: [v0.5.3...v0.6.0](https://github.com/limrun-inc/go-sdk/compare/v0.5.3...v0.6.0)

### Features

* **api:** add comma-separated state for multi-state listings ([983dbdc](https://github.com/limrun-inc/go-sdk/commit/983dbdcf3ba96ddd67612722f556b8bc5c38b600))
* **api:** add pagination for ios instances and assets as well ([ab0138c](https://github.com/limrun-inc/go-sdk/commit/ab0138ceca7c164dc08c2cb0d7487ddd35fdf3ee))
* **api:** add pagination to asset spec ([d32b77e](https://github.com/limrun-inc/go-sdk/commit/d32b77e501a2e12abbba5cab2fe0fa25a70f7b40))
* **api:** disable pagination for assets ([3007624](https://github.com/limrun-inc/go-sdk/commit/3007624de08e4813a3e753ed8a0c21b156d4b02f))
* **api:** enable pagination for android_instances ([f71655b](https://github.com/limrun-inc/go-sdk/commit/f71655b56deace52b8d0232f87f849b48bbd13eb))
* **api:** manual updates ([cf32b69](https://github.com/limrun-inc/go-sdk/commit/cf32b69d76451d1218e399d1ad7ae302f0848a06))
* **api:** manual updates ([daadc36](https://github.com/limrun-inc/go-sdk/commit/daadc3629072b1492e0509aaa36921e9a949f2b3))
* **api:** move pagination prop to openapi ([fb183b2](https://github.com/limrun-inc/go-sdk/commit/fb183b26d51dc2fdb5069c8ce013e61e07b09e94))
* **api:** regenerate new pagination fields ([c5c72bc](https://github.com/limrun-inc/go-sdk/commit/c5c72bc447f7ca7b7f2b894ae1fd72b631c97faf))
* **api:** update comment ([4a27752](https://github.com/limrun-inc/go-sdk/commit/4a277525beb8fa3a4c69b2af99b50c7055d62ce6))

## 0.5.3 (2025-11-05)

Full Changelog: [v0.5.2...v0.5.3](https://github.com/limrun-inc/go-sdk/compare/v0.5.2...v0.5.3)

### Features

* **api:** add asset deletion endpoint ([51f1b6b](https://github.com/limrun-inc/go-sdk/commit/51f1b6bf509fb68b4f3b36a7794d0cfc0c09784c))
* **api:** add the assigned state to both android and ios instance states ([b22d05e](https://github.com/limrun-inc/go-sdk/commit/b22d05ecade6f01d03020749d18fc4219b4f5c67))

## 0.5.2 (2025-11-04)

Full Changelog: [v0.5.1...v0.5.2](https://github.com/limrun-inc/go-sdk/compare/v0.5.1...v0.5.2)

### Features

* **api:** add launchMode to iOS asset object ([3e7e52b](https://github.com/limrun-inc/go-sdk/commit/3e7e52b60cc97437bd899a2006c5812dda7cb2f7))


### Chores

* **internal:** grammar fix (it's -&gt; its) ([9a41dcc](https://github.com/limrun-inc/go-sdk/commit/9a41dccda01041b8b3a50b64ccae6d7081abbc21))

## 0.5.1 (2025-10-31)

Full Changelog: [v0.5.0...v0.5.1](https://github.com/limrun-inc/go-sdk/compare/v0.5.0...v0.5.1)

### Features

* **api:** add ios port-forward endpoint url to return type ([0f1451f](https://github.com/limrun-inc/go-sdk/commit/0f1451f44876562d5f97addeca510f583ec4e38b))

## 0.5.0 (2025-10-29)

Full Changelog: [v0.4.2...v0.5.0](https://github.com/limrun-inc/go-sdk/compare/v0.4.2...v0.5.0)

### Features

* **api:** add explicit pagination fields ([ccfb77f](https://github.com/limrun-inc/go-sdk/commit/ccfb77fa7038030fa35b659e0d6861b43c61b88e))
* **api:** add os version clue ([0e1ab7f](https://github.com/limrun-inc/go-sdk/commit/0e1ab7f71dd62915731e152f5575d66d0abddc5e))
* **api:** limit pagination only to limit parameter temporarily ([aed4f11](https://github.com/limrun-inc/go-sdk/commit/aed4f11700c1456717167957204d75f141a15fc2))
* **api:** manual updates ([580e699](https://github.com/limrun-inc/go-sdk/commit/580e6999ce131e268ef52873e0dc0ce96562f205))
* **api:** manual updates ([8ef0cf6](https://github.com/limrun-inc/go-sdk/commit/8ef0cf67956a19b44952347e759be2b940429974))
* **api:** mark public urls as required ([ae0cb78](https://github.com/limrun-inc/go-sdk/commit/ae0cb78284cd5f52565bdc5974746cb2c65e4692))
* **api:** os version description to show possible values ([3801b23](https://github.com/limrun-inc/go-sdk/commit/3801b232cc406d3dfc1fcad6262295dcf47d410a))
* **api:** osVersion clue is available only in Android yet ([a2be626](https://github.com/limrun-inc/go-sdk/commit/a2be626adc9f9d1121797748def9eb415b2bdefa))
* **api:** remaining pieces of pagionation removed temporarily ([4059c88](https://github.com/limrun-inc/go-sdk/commit/4059c88d7ca1adbf8c1cf843a476ae8f4b17366a))
* **api:** revert api change ([ed7f2a6](https://github.com/limrun-inc/go-sdk/commit/ed7f2a65dc8a35851128b869282259615dcf7020))
* **api:** update assets and ios_instances endpoints with pagination ([ddd136b](https://github.com/limrun-inc/go-sdk/commit/ddd136bfe89fb384fca499c0a622e3712c2002d2))
* **api:** update stainless schema for pagination ([e53151e](https://github.com/limrun-inc/go-sdk/commit/e53151e9a6f895d40eb454d0c899b3b2b7dde47a))

## 0.4.2 (2025-10-06)

Full Changelog: [v0.4.1...v0.4.2](https://github.com/limrun-inc/go-sdk/compare/v0.4.1...v0.4.2)

### Features

* **api:** add the new multiple apk installation options ([0d3233b](https://github.com/limrun-inc/go-sdk/commit/0d3233b091b86a923ef572889ac917ac91b2b337))


### Bug Fixes

* bugfix for setting JSON keys with special characters ([a47cddf](https://github.com/limrun-inc/go-sdk/commit/a47cddfbe787d753c8d703e01e51f3acb167afae))
* use slices.Concat instead of sometimes modifying r.Options ([d28471e](https://github.com/limrun-inc/go-sdk/commit/d28471ee3e1133033e2a60656e86e8578d88fcf8))


### Chores

* bump minimum go version to 1.22 ([6118da1](https://github.com/limrun-inc/go-sdk/commit/6118da1a28b0abfd775ba62b928c70e88f4a5992))
* do not install brew dependencies in ./scripts/bootstrap by default ([46d3bb2](https://github.com/limrun-inc/go-sdk/commit/46d3bb2dc3f056d9764fb910ef8454ec19d9dab2))
* update more docs for 1.22 ([cfda39e](https://github.com/limrun-inc/go-sdk/commit/cfda39eabdfa379f8c7323b266e36cffcccca252))

## 0.4.1 (2025-09-18)

Full Changelog: [v0.4.0...v0.4.1](https://github.com/limrun-inc/go-sdk/compare/v0.4.0...v0.4.1)

## 0.4.0 (2025-09-12)

Full Changelog: [v0.3.0...v0.4.0](https://github.com/limrun-inc/go-sdk/compare/v0.3.0...v0.4.0)

### Features

* **api:** manual updates ([6d1a15b](https://github.com/limrun-inc/go-sdk/commit/6d1a15b4f6306f1aa3b0141d127c9e5b936fd863))
* **api:** manual updates ([d48432f](https://github.com/limrun-inc/go-sdk/commit/d48432ffefbe7deb632923ae9f7d2366c2ce2a1e))

## 0.3.0 (2025-09-11)

Full Changelog: [v0.2.0...v0.3.0](https://github.com/limrun-inc/go-sdk/compare/v0.2.0...v0.3.0)

### Features

* **api:** add typescript ([cf4d01c](https://github.com/limrun-inc/go-sdk/commit/cf4d01c86ff162864e552a50174f1179a9ac5fa4))
* **api:** remove md5filter from list assets ([b2cbdde](https://github.com/limrun-inc/go-sdk/commit/b2cbdde31120941ed20885d219c5682f45b306d6))
* **api:** rename retrieve to get ([ba13401](https://github.com/limrun-inc/go-sdk/commit/ba13401c798500501c30eb4fdd035c275e7b901a))

## 0.2.0 (2025-09-08)

Full Changelog: [v0.1.0...v0.2.0](https://github.com/limrun-inc/go-sdk/compare/v0.1.0...v0.2.0)

### Features

* **assets:** add getOrUpload method and example ([1debc0c](https://github.com/limrun-inc/go-sdk/commit/1debc0cae08deb62f7e9a5335f7b182d94b668b8))
* **assets:** rename create to getOrCreate ([2df7ff5](https://github.com/limrun-inc/go-sdk/commit/2df7ff5817290e496f848bcc8ad0c0dd1a8732b1))
* **examples:** add server example ([0d5b692](https://github.com/limrun-inc/go-sdk/commit/0d5b692334b3b3da0bc4351d4ccbc7c3a0584946))
* **tunnel:** add tunnel package and example ([4e84071](https://github.com/limrun-inc/go-sdk/commit/4e840719bdc54e0d15d96e7ba0857f2cc2b8f5a4))

## 0.1.0 (2025-09-08)

Full Changelog: [v0.0.1...v0.1.0](https://github.com/limrun-inc/go-sdk/compare/v0.0.1...v0.1.0)

### Features

* **api:** manual updates ([e66fbf6](https://github.com/limrun-inc/go-sdk/commit/e66fbf665558ec3fb1cf52ed2d02630bd6807617))
* **api:** manual updates ([41c8d8a](https://github.com/limrun-inc/go-sdk/commit/41c8d8a105dbd196270fec13d72348a57ea84c2b))


### Chores

* update SDK settings ([e1fe11c](https://github.com/limrun-inc/go-sdk/commit/e1fe11c0ac894be47fba0aeed7106bdd13c92b57))
