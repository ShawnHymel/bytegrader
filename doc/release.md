# How to Release a New Version (Maintainers Only)

1. Review and consolidate "Unreleased" changes in CHANGELOG.md
2. Assign appropriate version number using [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
3. Update VERSION file
4. Commit changes directly to main: `git commit -m "Prepare release vX.Y.Z"`
5. Create and push git tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
6. Create GitHub release from the tag
