# Air Cover

Air Cover is a Spinitron integration that allows DJs (Personas) to request substitutes to cover their shows, and in turn, allows them to pick up other substitution requests.


## Architecture


### Hexagonal diagam

```mermaid
flowchart LR
    user["DJs and admins"] --> router["HTTP router and OpenAPI validation<br/>internal/cmd"]
    router --> http["Inbound HTTP adapter<br/>internal/adapters/http"]

    subgraph core["Application core"]
        direction TB
        services{{"Application services<br/>auth, subrequests, admin, bootstrap"}}
        domain["Domain model<br/>users, sessions, sub requests"]
        ports["Ports<br/>Repository, Sender, Catalog"]

        services --> domain
        services --> ports
    end

    http --> services
    ports --> sqlite["SQLite repository adapter<br/>internal/adapters/sqlite"]
    ports --> email["Email sender adapter<br/>internal/adapters/email"]
    ports --> spinitron["Spinitron catalog adapter<br/>internal/adapters/spinitron"]

    sqlite --> db[("SQLite database")]
    email --> sendgrid["SendGrid API<br/>or console sender"]
    spinitron --> spinitronApi["Spinitron API"]
```

### File organization

This project is built using Go. The general structure follows standard Go project layouts:

- `cmd/aircover/`: Contains the main application entry point.
- `internal/`: Contains private application code.
  - `internal/domain/`: Domain entities and behavior (users, auth, substitution requests, etc.)
  - `internal/app/`: Application services and ports.
  - `internal/adapters/`: Outbound adapters for SQLite, email, and Spinitron.
  - `internal/adapters/http/`: Handlers for the application's HTTP API.
