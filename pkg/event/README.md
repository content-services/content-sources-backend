# package pkg/event

This package hold all the code related with asynchronous
event communication, making this more reusable, or candidate
to be transformed into a module in the future.

In relation with the client library configuration, the following
link provide all the information that can be configured:

https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md

The content is structured as below:

```raw
pkg/event
├── adapter
├── handler
├── schema
└── message
```

* **adapter**: It contains all the port in and port out adapters.
  Every adapter is directly related with an event message, but the
  one for the kafka headers, and it own as many methods as transformations
  for the port in or port out adapters. The external dependencies are
  centralized here, the rest of the package use functions, structs and
  interfaces declared at `pkg/event`.
* **handler**: Any message handler is defined here.
* **schema**: Only contains the schema definition for the
  messages. The message structures are generated from this
  definition. We define the schema following
  [this specification](https://json-schema.org/specification.html).
  The `schema.go` file define structs and validation methods which
  will allow to validate a message before to produce it, and before
  it is consumed by the handler.
* **message**: Auto-generated code which represent the golang
  struct for the message produced and consumed in relation to
  content-service.

## Adding a new event message

> TODO It is necessary a special handler to route message to
> several handlers; otherwise, several run loops will be running.

* Define the schema for the message at `pkg/event/schema`.
* Generate structs by `make gen-event-messages`; this will
  generate a new file at `pkg/event/message` directory.
* If you are consuming the message, add your handler at
  `pkg/event/handler` directory.
* If you need some adapter port input or output, add a new
  file associated to the message at `pkg/event/adapter`.
  * If you need to transform to the new message struct,
    add a new interface with the `PortIn` prefix.
  * If you need to transform from the new message struct,
    add a new interface with the `PortOut` prefix.
* Add unit tests for each new generated component.
