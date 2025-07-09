# How to Release a New Version (Maintainers Only)

1. Update main: 

```sh
git checkout main
git pull
```

2. Choose appropriate version number using [Semantic Versioning](https://semver.org/spec/v2.0.0.html)
3. Review and consolidate `Unreleased` changes in **CHANGELOG.md**, assign new version number to the section
4. Update **VERSION** file with new version number
5. Commit changes directly to main (you must be on the maintainers bypass list to push to main):

```sh
git add CHANGELOG.md VERSION
git commit -m "Prepare release vX.Y.Z"`
git push origin main
```

6. Create and push tag:

```sh
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

7. Create a GitHub release:

```sh
gh release create vX.Y.Z --title "vX.Y.Z" --notes "See [CHANGELOG.md](CHANGELOG.md) for release details."
```
