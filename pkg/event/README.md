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
  Every adapter is directly related with an event message,
  and it own as many methods as transformation for the
  port in or port out adapters.
* **handler**: Any message handler is defined here.
* **schema**: Only contains the schema definition for the
  messages. The message structures are generated from this
  definition. We define the schema following
  [this specification](https://json-schema.org/specification.html).
* **message**: Auto-generated code which represent the golang
  struct for the message produced and consumed in relation to
  content-service.
