# package pkg/event

This package holds all the code related with asynchronous
event communication, making this more reusable, and a candidate
to be transformed into a module in the future.

In relation with the client library configuration, the following
link provides all the information that can be configured:

https://github.com/edenhill/librdkafka/blob/master/CONFIGURATION.md

The content is structured as below:

```raw
pkg/event
├── adapter
├── handler
├── schema
└── message
```

* **adapter**: It contains all the port-in and port-out adapters.
  Every adapter is directly related with an event message, (but the
  one for the kafka headers), and it has as many methods as transformations
  for the port in or port out adapters. The external dependencies are
  centralized here, the rest of the package use functions, structs and
  interfaces declared at `pkg/event`.
* **handler**: Any message handler is defined here.
* **schema**: Only contains the schema definition for the
  messages. The message structures are generated from this
  definition. We define the schema following
  [this specification](https://json-schema.org/specification.html).
  The `schema.go` file define structs and validation methods which
  will enable validating a message before producing it, and before
  it is consumed by the handler.
* **message**: Auto-generated code which represent the golang
  struct for the message produced and consumed in relation to
  content-service.

## Adding a new event message

* Define the message schema at `pkg/event/schema` (yaml file).
* Add the new schema content as a local
  `schemaMessage<MySchema>` variable at
  `pkg/event/schema/schemas.go` file.
* Add a new `Topic<MySchema>` constant that represent your schema.
* Add the above constant to the `AllowedTopics` slice.
* Generate structs by `make gen-event-messages`; this will
  generate a new file at `pkg/event/message` directory.
* If you are consuming the message, add your handler at
  `pkg/event/handler` directory.
* If you need some adapter input or output port, add a new
  file associated to the message at `pkg/event/adapter`.
  * If you need to transform to the new message struct,
    add a new interface with the `PortIn` prefix.
  * If you need to transform from the new message struct,
    add a new interface with the `PortOut` prefix.
* Add unit tests for each new generated component.

## Adding a producer

If you need to produce one new message, follow the above steps
but instead of create an event handler, add a new producer at
`pkg/event/producer/` directory, similar to the
`pkg/event/producer/introspect_request.go` file.

* Add interface `<MyTopic> interface`.
  * It contains `Produce(ctx echo.Context, msg *message.<MyTopic>Message) error`.
* Add specific type `<MyTopic>Producer struct`. It contains as minimum
  the *kafka.Producer.
* Add `New<MyTopic>(producer *kafka.Producer, ...) (<MyTopic>, error)` function.
* Implement your `Produce` method.

## Debugging event handler

* Prepare infrastructure by: `make db-clean kafka-clean db-up kafka-up`
* Import repositories by `make repos-import`
* Add breakpoints and start your debugger.
* Produce a demo message, for instance: `make kafka-produce-msg-1`
* Happy debugging.

## Debugging producer

Currently producer is launched at the end of some http api handler.

* Prepare infrastructure by: `make db-clean kafka-clean db-up kafka-up`
* Import repositories by `make repos-import`
* Start debugger from your favoruite IDE, set a breakpoint
  into the handler, or directly into your `Produce` method for
  your message producer.
* Send a request with a valid payload for the API call.
* Happy debugging.
