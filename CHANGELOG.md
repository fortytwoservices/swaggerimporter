# Changelog

## 1.0.0 (2025-12-19)


### Features

* add clustered ([6d98df7](https://github.com/fortytwoservices/swaggerimporter/commit/6d98df7e31ca242aaa8ce13e989937c6a4d0267d))
* add safe version parsing with bounds checking ([165ac60](https://github.com/fortytwoservices/swaggerimporter/commit/165ac60ce1eea66f2a3e81735335bf4f2d729f6f))
* add support for crossplane v2 ([223be2f](https://github.com/fortytwoservices/swaggerimporter/commit/223be2f8aa635e9c8b0983d31c62c0f7a2134a40))
* add support for crossplane v2 ([ed25480](https://github.com/fortytwoservices/swaggerimporter/commit/ed254803b880647f5e1989b1ceec7246a8fe6b77))
* added controller, dockerfile and test deployment manifest ([c247b2f](https://github.com/fortytwoservices/swaggerimporter/commit/c247b2fafd3f3509a99d90674e040521aeff9a3a))
* added controller, dockerfile and test deployment manifest ([342b93e](https://github.com/fortytwoservices/swaggerimporter/commit/342b93ed565706351537e127b3dd6ee1992cd14c))
* create ingress based on latest api version available ([9294fb4](https://github.com/fortytwoservices/swaggerimporter/commit/9294fb46b0d70100a89e9526b25a65c41d500288))
* create ingress based on latest api version available ([3e01961](https://github.com/fortytwoservices/swaggerimporter/commit/3e019610e0330e1f24e7a1b5fcc54aa9daaf7419))
* only reconcile application if it has label swaggerimporter: true ([593bbb6](https://github.com/fortytwoservices/swaggerimporter/commit/593bbb6759ae87168fc9e39d39ab901e3c0e65cf))
* only reconcile application if it has label swaggerimporter: true ([3b72b5b](https://github.com/fortytwoservices/swaggerimporter/commit/3b72b5b19682158202ab56638327ff5dea92b9ea))
* set check on already updated resources to 60 ([1fb135c](https://github.com/fortytwoservices/swaggerimporter/commit/1fb135c27ebc2b2b058cd34612076bc5a5614713))
* update dockerfile to use base version of 1.23 golang ([f8e4129](https://github.com/fortytwoservices/swaggerimporter/commit/f8e4129cfc01914b26659863da8b3062aae72db9))
* update dockerfile to use base version of 1.23 golang ([6db96e1](https://github.com/fortytwoservices/swaggerimporter/commit/6db96e1d6808a0e9b3a3c4642a8f9c22ff9d9bf1))
* update how often the operator reconciles ([cdfe59d](https://github.com/fortytwoservices/swaggerimporter/commit/cdfe59d1daf82d6ab7ede89d968f1a5ad3e276bc))
* update to not requeue pods if they no longer exist ([fe04c44](https://github.com/fortytwoservices/swaggerimporter/commit/fe04c442aaf79a3cdcf8ab43e6c715f73406dc1d))
* update to not requeue pods if they no longer exist, to remove not found pods errors from container logs ([7f5d44b](https://github.com/fortytwoservices/swaggerimporter/commit/7f5d44b7494ee0ffc4eceb52c5f230098495c565))
* update to use correct ingress name and hostnames with identifiers to work with two clusters ([2593c2b](https://github.com/fortytwoservices/swaggerimporter/commit/2593c2b7895b5ce000732df9bea5facc890230a2))
* use dns names instead of port-forward ([6ad8f10](https://github.com/fortytwoservices/swaggerimporter/commit/6ad8f101130425e67b115446a3fc22ffd23fceea))
* use dns names instead of port-forward ([3277e72](https://github.com/fortytwoservices/swaggerimporter/commit/3277e7222f3b71e04f556b61f9eafb74936908bd))


### Bug Fixes

* close response body immediately to prevent resource leak ([2b15e13](https://github.com/fortytwoservices/swaggerimporter/commit/2b15e13e908f17a6c10003763e09ad23a67efb76))
* correct build ([4c34ca2](https://github.com/fortytwoservices/swaggerimporter/commit/4c34ca241913b0b3776c1564541efdd11c14feef))
* correct err ([fbec60d](https://github.com/fortytwoservices/swaggerimporter/commit/fbec60d30a289a406864a8317fbefaf30bd0432c))
* correct testing ([c262634](https://github.com/fortytwoservices/swaggerimporter/commit/c262634bc0b441ac991aca046cf572b8389bd3e8))
* correct typo ([c893d12](https://github.com/fortytwoservices/swaggerimporter/commit/c893d12b41dafc30d37203790dbb520721945ff2))
* correct unit test ([f4539b2](https://github.com/fortytwoservices/swaggerimporter/commit/f4539b2debf0b63234daeffe2619fe44e3bfae16))
* ensure defer runs after each loop iteration to prevent resource leaks ([d4aa901](https://github.com/fortytwoservices/swaggerimporter/commit/d4aa9017949c77e3afce77ecac545a6bea245469))
* explicitly pass empty string for cluster-scoped API namespace ([b85df43](https://github.com/fortytwoservices/swaggerimporter/commit/b85df4354e4b7a8d91efc15058fb1d754e8b4448))
* remove unused role ([f3587bc](https://github.com/fortytwoservices/swaggerimporter/commit/f3587bc0e0fd42d26013f3a741aeacca775eefc7))
* use defer to close HTTP response body immediately after request ([19cc44c](https://github.com/fortytwoservices/swaggerimporter/commit/19cc44cfb14800450c02453aff2bdf9f0ab0b899))
