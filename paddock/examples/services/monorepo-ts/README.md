# TypeScript workspace fixture

This is a deliberately small monorepo-shaped subject. The root `tsconfig.json`
maps package-style aliases across `packages/shared` and `packages/orders`.
The good variant keeps the dependency direction inward; the violating variant
makes shared code depend on the orders domain.
