# Some Random Notes 

## 1. `go.mod` file is like package.json of node projects. Contains:
    - Module path: Your project's import path (e.g., module github.com/user/project)
    - Go version: Minimum Go version required
    - Dependencies: Direct dependencies with versions

```go
    module github.com/example/myproject

    go 1.21

    require (
        github.com/gin-gonic/gin v1.9.1
        google.golang.org/grpc v1.59.0
    )

```

## 2. `go.sum` The checksum/lock file - similar to package-lock.json in Node.js. Contains:
    - Cryptographic hashes of each dependency (and their go.mod files)
    - Ensures reproducible builds - verifies downloaded modules haven't been tampered with
    - Includes both direct and transitive dependencies

```
  github.com/gin-gonic/gin v1.9.1 h1:4idEAncQnU5cB7BeOkPtxjfCSye0AAm1R0RVIqFPSsg=
  github.com/gin-gonic/gin v1.9.1/go.mod h1:hPrL7YrpYKXt5YId3A/Tn+7XD...
```

## 3. Key Differences

    | Aspect             | go.mod               | go.sum              |
    |--------------------|----------------------|---------------------|
    | Purpose            | Declare dependencies | Verify integrity    |
    | Edit manually?     | Yes                  | No (auto-generated) |
    | Commit to git?     | Yes                  | Yes                 |
    | Node.js equivalent | package.json         | package-lock.json   |

## 4.  Common Commands
```
  go mod init <module-name>  # Create go.mod
  go mod tidy                # Add missing / remove unused deps
  go get <package>           # Add a dependency
```