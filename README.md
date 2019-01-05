# ecslog

ecslog is an experimental structured logger for the Go programming language.

Aim of this project is to create a type safe logger generating log events which
are fully compatible to the [Elastic Common Schema
(ECS)](https://github.com/elastic/ecs). ECS defines a common set of fields for
collecting, processing, and ingesting data within the [Elastic Stack](https://www.elastic.co/guide/en/elastic-stack/current/elastic-stack.html#elastic-stack).

Logs should be available for consumption by developers, operators, and any kind
of automated processing (index for search, store in databases, security
analysis).

While developers might want add some additional internal states to log messages
for being able to troubleshoot issues, other users might not gain much value
from unexplained internal state values. We should aim for human readable and
self-explanatory messages which do not require the additional context added by
developers in order to make sense.  Yet in the presence of micro-services and
highly multithreaded applications standardized context information is mandatory
for filtering and correlating relevant log messages by machine, service,
thread, API call or user.

Ideally automated processes should not have to deal with parsing the actual
message. Messages can easily change between releases, and should be ignored at
best. We can and should provide as much insight into our logs as possible with
the help of structured logging.

With most structured logging libraries just outputting everything to JSON as
is, developers can easily change a field name or the type of a field without
anyone noticing. In bigger projects there is also the chance of developers
using the same field names, but with values of different types. These
undetected schema changes and incompatibilities put automation at risk of
breaking if application are updated:
- A subset of logs might not be indexible in an Elasticsearch Index anymore due
  to mapping conflicts for example.
- Scripts/Applications report errors or crash due to unexpected types
- Log analysers produce wrong results due to expected fields missing

Creating logs based on a common schema like ECS helps in defining and
guaranteeing a common log structure a many different stakeholders can rely on
(See [What are the benefits of using ECS?](https://github.com/elastic/ecs#what-are-the-benefits-of-using-ecs)).
ECS defines a many common fields, but is still extensible (See
[Fields](https://github.com/elastic/ecs#fields)). ECS defines a core level and
an extended level, [reserves some common
namespaces](https://github.com/elastic/ecs#reserved-section-names). It is not
fully enclosed, but meant to be extended, so to fit an
applications/organizations needs.

ecslog distinguishes between standardized and user(developer) provided fields.
The standardized fields are type-safe, by providing developers with type-safe
field constructors. These are checked at compile time and guarantee that the
correct names will be used when serializing to logs to JSON.

ECS [defines its schema in yaml files](https://github.com/elastic/ecs/tree/master/schemas).
These files are compatible to `fields.yml` files being used in the Elastic
Beats project. Among others Beats already generate Documentation, Kibana index
patterns, [Elasticsearch Index Templates](https://www.elastic.co/guide/en/elasticsearch/reference/current/indices-templates.html)
based on these definitions.

ecslog reuses the definitions provided by ECS, so to generate the code for the
type-safe ECS compatible field constructors (See [tool
sources](https://github.com/urso/ecslog/tree/master/fld/ecs/internal/cmd/genfields)).


## Structure/Concepts

**Packages**:
- **.**: Top level package defining the public logger.
- **./backend** logger backend interface definitions and composable implementations for building actual logging outputs.
- **./ctxtree**: internal representation of log and error contexts.
- **./fld**: Support for fields.
- **./fld/ecs**: ECS field constructors.
- **./errx**: Error support package with support for:
  - wrapping/annotating errors with additional context
  - querying errors by predicate, contents, type
  - walking trace/tree of errors

### Fields

ecslog distinguishes between standardized fields and user fields. We provide
type safe constructors for standardized fields, but user defined fields are not
necessarily type-safe and often carry additional debug information for
consumption by the actual developer. Consumers of logs should be prepared to
remove user fields from log messages if necessary.

The structured logging backends mix in standardized fields as is, right at the
root of the document/event to be generated. User fields are prefixed by
`fields.`.

This log statement using the standardized `ecs.agent.name` field and the user defined `myfield`:

```
	log.With(
		"myfield", "test",
		ecs.Agent.Name("myapp"),
	).Info("info message")
```

produces this JSON document:

```
    {
      ...
      "agent": {
        "name": "myapp"
      },
      "fields": {
        "myfield": "test"
      },
      "log": {
        ...
      },
      "message": "info message"
    }
```

### Context

The logger it's context is implemented by the **ctxtree** package.
Fields can only be added to an context, but not be removed or updated. Adding
the same field twice or to a derived context will 'shadow' the original field,
but not removing it.

A field added twice to a context will be reported only once, ensuring tools
processing the produced JSON document always see a well defined JSON document
(this is not the case with all structured logging libraries).

Calling: 

```
	log.With("field", 1, "field", 2).Info("hello world")
```

or:

```
	log.With("field", 1).With("field", 2).Info("hello world")
```

produces:

```
    {
      ...
      "fields": {
        "field": 2
      },
      "log": {
        ...
      },
      "message": "hello world"
    }
```

The context is represented as a tree. Within one node in the context fields are
ordered by the order they've been added to the context.
When creating a context, one can pass a 'predecessor' and a 'successor' to the
context. A snapshot of the current state of these contexts will be used, so to
allow concurrent use.

The order of fields in a context-tree is determined by an depth-first traversal
of all contexts in the tree. This is used to link contexts between loggers 
top-down, while linking contexts of error values from the bottom upwards in the stack.

### Format strings

The logging methods `Trace`, `Debug`, `Info`, `Error` always assume a format
string is given. There are no alternative definitions not accepting a format
string as first parameter. The intend of these methods is to create readable
and explanatory messages.

The format strings supported are similar to the fmt.XPrintf family, but add
support for capturing additional user fields in the current log context:

```
	log.Error("Can not open '%{file}'.", "file.txt")
```

produces this document:

```
{
  ...
  "fields": {
    "file": "file.txt"
  },
  "log": {
    ...
  },
  "message": "Can not open 'file.txt'."
}
```


Applications should log messages like `"can not open file.txt"` instead of
`"can not open file"` asking the user to look at configuration or some context
fields in the log message. ecslog provides support to backends to supress the
generation of the context. The text backend without context capturing will just print:

```
2019-01-05T20:30:25+01:00 ERROR	main.go:79	can not open file.txt
```

Standardized fields can also be passed to a format string via:

```
	log.Error("Failed to access %v", ecs.File.Path("test.txt"))
```

or:

```
	log.Error("Failed to access '%{file}'", ecs.File.Path("test.txt"))
```

Both calls produce the document:

```
{
  ...
  "file": {
    "path": "test.txt"
  },
  "log": {
    ...
  },
  "message": "Failed to access 'test.txt'"
}
```

### Errors

