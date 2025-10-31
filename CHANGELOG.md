# Changelog

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
