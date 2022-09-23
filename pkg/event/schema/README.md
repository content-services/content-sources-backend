# Proof of concept for event message handlings

```raw
pkg/
└── event
    ├── message  ## Do not modify this directory, it is self-generated from schema directory
    ├── schema   ## Hold the message schemas defined
    │   ├── header.message.yaml               ## Define schema for the header ## TODO probably to be removed
    │   ├── introspectRequest.message.yaml    ## Define schema for the message payload
    │   └── schemas.go  ## Schema loader which prepare all the necessary message schemas
```

