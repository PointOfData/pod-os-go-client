## Neural Memory Database Manages Memory Events and Relationships
Pod-OS, uniquely, provides a Neural Memory Database that has the following behavioral characteristics: acquiring (encoding), stabilizing (consolidation), and retrieving information, and forming engrams (memory traces) within groups of Events. Key characteristics include its large storage capacity with decreasing marginal storage volume, native version management learning and ease, and ability to inter-link and describe any and all Event Objects using weights or activation functions, action policies, or other objective function methods. The Neural Memory DB's central thesis is that each moment in time captures, and processes important context that software, and the most advanced, complex and adaptive AIs and robotics require. Including LLMs, RAG, robotics, World Models, and other advanced AI and robotics applications. Neural Memory provides capture three memory behaviors that are interrelated: long-term memory (facts and experiences), short-term memory (actions, rewards, variables, policies, cognition, reasoning and beliefs),  and real-time state (continuous snapshot of necessary info (for example, positions, sensor readings, game status) for decision-making)). Is is a natural fit for Neuro-Symbolic AI, and other memory structures. 

The Neural Memory Database uses three primitives: 
1. Event Objects:An Event Object is a uniquely identified datum (from any source) which is used as a base reference for a set of versioned attributes and context bound weighted links to other event objects. Event Objects carry crucial context information including time and location, owner, encryption, message source as actor@gateway, message destination actor@gateway, and payload MIME and binary data. 
2. Tags: Tags are values that describe important information about the Event. There are multiple design options. The simplest design option is to use frequency=tagvalue. Frequency is an integer that describes the number of times the Tag has been applied to the Event. Tagvalue is a string or integer or float or embedding or binary data or other type that describes the value of the Tag. A powerful design option is to use Facets which are key/value pairs of any type in the format of frequency=key_name=key_value. More complex designs are possible. For example, a tag value can be a hash of the Event Object, a pointer to the Event Object, a binary data blob or a hierarchical string or integers or floats or embeddings or other type that describes the value of the Tag. What is required is that the Tag value can be searched for and retrieved using the Tag value following the guidance in the Retrieval/Search Guidance section.
3. Link Objects: Links connect any two Events (including Link Event), but as Links are Events themselves, also carry the Event Object and Tag descriptions along with weights (integer or continuous function). 

Neural Memory uses Tags to implicitly associate events by Tag name, time, and location. Neural Memory uses links to explicitly associate events. 

By combining Links and Tags, complex data structures and sets of related events can be created depending on the objective. For example, the Neural Memory Database can capture neural memories and semantic memories; can use Tags and Links to form short and long-term memories; and can use Links and Tags to form short and long-term relationships between events. In other data storage examples, the Neural Memory Database can be used to create a knowledge graph, a semantic network, or a graph of related events; the Neural Memory Database can also be used to mimic the behavior of a relational database such as SQL, a document database, or a graph database. In other examples, the Neural Memory Database can be used to encode in data structures a neural network (e.g., FNN, CNN, RNN, LSTM, GRU), transformers, a genetic algorithm, or a reinforcement learning model.

Links and tags can be defined independently of the database actually containing the original event definition, permitting the distribution of data across servers or database files. Retrieval of the actual event may require a request to other database handlers (or a router which understands that event requests need to be distributed to a given set of handlers), but there is no requirement for the actual event to be recorded within a database in which tags are placed. While this can cause data concurrency challenges, it also permits explicit segregation where it is required (such as a security application, or private analytical data associated with public information).

Pod-OS uses specific Types [podos/types] to manage communication with the Actor. All Event Message interactions use a Request/Response pair either in synchronous (STREAMING ON) or asynchronous mode (STREAMING OFF). The Actor always returns a Response to the client's request. Depending on the implementation, the client may choose to use the synchronous or asynchronous mode; and needs to handle Message management accordingly.

The Actor Response carries crucial information that may be remembered depending on the operation. 

### Store Event Object Efficiency Guidance
- When multiple Event Objects need to be created, strongly prefer to use StoreBatchEvents Intent as it is more efficient; this can also accept Tags as part of the batch.
- When using StoreBatchEvents, optimize batch size to minimize network overhead and latency; Neural Memory DB's storage performance does not increase linearly with batch size;storage performance is best with larger batch sizes (e.g., 10,000 - 100,000 events per batch). On a single board computer with 1GB of RAM, the optimal batch size is 10,000 events; setting a response timeout of 3 minutes.
- When a single Event Object is created, prefer to add the Tags in StoreEvent Intent as it is more efficient. 
- When adding Tags prefer to useStoreBatchTags Intent. 


### Retrieval/Search Guidance
... 

### Ownership Guidance
Ownership is overloaded with two meanings: 
1. The EventOwner is the EventId or EventUniqueId of the representing the entity that created the Event Object. 
2. RBAC (Role-Based Access Control) ownership is defined elsewhere and is not part of the Event Owner.

### Reference Guidance
- Internally, all communication about Event Objects and Link Objects uses the EventId or EventUniqueId field for reference. The EventId is created by the Actor when the Event Object is stored. As such, the EventId is the primary reference for all Event Objects and Link Objects. EventUniqueId is a very useful developer-set customer ID for external reference by the developer.
- MessageId can be is used by client applications to track the message and conversation flow. 

### Design Patterns
These are optional design patterns that can be used to create complex data structures and sets of related events.
- Sharding: Separate Event Objects into each shard using Routing rules, retrieve by filtering on Tag values using GetEventsForTags Intent for each Service shard.
- Replication: Duplicate Event Objects by creating a Link Object to the original Event Object.
- Relational data structure: a relational database table can be simulated by creating a table Event Object, and an Event Object for each row, and for each row a series of column Event Objects. Associate all Event Objects together in a hierarchy using Link Objects.  Create Links to the table from each row, and from the events making up the columns in a specific row to that row event. Since links are themselves a special type of event, they can be assigned tag values and linked to other events.
- Retrieval Augmented Generation: [to be completed]
- Knowledge Graph: [to be completed]

### Storing Events
Events are stored as a single Event Object [StoreEvent Intent type] or as a batch of Event Objects [StoreBatchEvents Intent type]. 

#### Rules:

Event Creation must follow these rules.

- The Event object must exist in the Neural Memory database store before any associated StoreBatchTags Intents or LinkEvents Intents are created.
- The definition of exist: the Actor responded with status 'OK' when the Event Object is stored or the Event Object is retrieved.
- For any EventOwner other than '$sys' (representing system-level creation) the EventOwner EventId or EventUniqueId must exist in the Neural Memory database; the Event Owner is only the EventId provided in the Response. 
- The EventOwner may be an internal EventID or an EventUniqueId returned by the Actor's response (found in decodedMessage). MessageId are not used. MessageId is used for tracking the message and conversation flow.
- EventUniqueId is a useful developer-set customer ID for external reference by the developer.

The EventId returned from the Actor is formatted using the system delimiter, which is ASCII character 1 (0x01). The format is always: "timestamp delim loc segment 1 delim loc segment 2 delim .... loc segment N"

The timestamp is always the number of seconds and microseconds since Jan 1, 1970 (a negative value indicates a time prior to this date). The location segments are always in the order supplied by the event creator.

When EventId is  sent as part of a Request, the format is always the same as Actor's Response EventId. 
The time stamp is always formatted as a 16-position floating-point value, where there are ten digits to the left of the decimal and six to the right, and a sign indicator is always used (positive
or negative). Both the decimal and whole number portions are prefixed with zeros as necessary.

The PayloadData may be any data to be stored as part of the event, up to 2 GB in size. This data may not be altered in the future.

#### Example Pod-OS Store Event Format:
```
Message{
		To:                     "administration@zeroth." + domainName, // The recipient actor@gateway
		From:                   "Pod-OSDashboardNode@zeroth." + domainName, // The sender in the form client@gateway. This field must contain the same client and gateway name used to establish the connection.
		ClientName:             "Pod-OSDashboardNode", // The client name, must be the same name used when establishing the connection. 
        MessageId:              [GUID] // A unique identifier for the message to track the message and conversation flow.
		Intent:                 IntentType.StoreEvent, // The intent of the message to store the event object.
		EventOwner:             [EventID from the Base Event], // The owner of the event object.
		EventUniqueId:          [GUID], // A unique identifier for the event object.
		EventTimestamp:         message.GetTimestamp(), // The timestamp of the event object.
		EventLocation:          "TERRA|47.619463|-122.518691", // The location of the event object; can be geospatial, temporal, or other.
		EventLocationSeparator: "|",
		EventType:              "system log object", // String that describes the type of event object.
		PayloadMimeType:        "text/plain", // The MIME type of the payload data.
		PayloadData:            "System initialization log", // The data of the event object.
		PayloadDataType:        RAW, // The data type of the payload data.
	}
```

#### Example Pod-OS StoreBatchEvents Format:
```
textPayload := []BatchEventSpec{
		BatchEventSpec{
			EventUniqueId:          [GUID], // A unique identifier for the event object.
			EventOwner:             [EventID or EventUniqueId from the Event representing the creator of the event object], // The creator of the event object.
			EventTimestamp:         message.GetTimestamp(), // The timestamp of the event object.
			EventLocation:          "TERRA|47.619463|-122.518691", // The location of the event object; can be geospatial, temporal, or other.
			EventLocationSeparator: "|", // The separator for the event location.
		},
	}
Message{
		To:                     "administration@zeroth." + domainName, // The recipient actor@gateway
		From:                   "Pod-OSDashboardNode@zeroth." + domainName, // The sender in the form client@gateway. This field must contain the same client and gateway name used to establish the connection.
		ClientName:             "Pod-OSDashboardNode", // The client name, must be the same name used when establishing the connection.
		MessageId:              [GUID], // A unique identifier for the message to track the message and conversation flow.
		Intent:                 IntentType.StoreBatchEvents, // The intent of the message to store the batch of event objects.
		PayloadMimeType:        "text/plain", // The MIME type of the payload data.
		PayloadData:            textPayload, // The data of the batch of event objects.
		PayloadDataType:        RAW, // The data type of the payload data.
	}
```

The text payload contains a series of newline-terminated records, each defining a separate event. Each line must be formatted as fieldname=value <tab> fieldname=value ... with the following required fields:

- EventUniqueId: A developer-provided unique ID for the event
- EventOwner OR EventOwnerUniqueId: The creator of the event object (use "$sys" for system-level creation) 
- EventTimestamp: The timestamp of the event (RFC3339 format recommended)
- EventLocation: The location of the event; developer defined
- EventLocationSeparator: The separator for the event location (default is "|")
- EventType: Developer-defined event type string

Optionally, the text payload can carry Tags. The tags are appended to the same line, tab-separated, as the event specifications each formatted as unique_tag_number=freq:tag_value.

Facets are a variation of tagvalue which allow greater flexibility and improved search. Their pattern, exclusively for StoreBatchEvents, is:
tag_unique_to_payload_number=frequency:key_name=key_value

For example:
```
EventUniqueId=550e8400-e29b-41d4-a716-446655440000	EventOwner=$sys	EventTimestamp=2024-01-15T10:30:00Z	EventLocation=TERRA|47.619463|-122.518691	EventLocationSeparator=|	EventType=system log object
EventUniqueId=550e8400-e29b-41d4-a716-446655440001	EventOwner=$sys	EventTimestamp=2024-01-15T10:31:00Z	EventLocation=TERRA|47.619463|-122.518691	EventLocationSeparator=|	EventType=system log object
EventUniqueId=550e8400-e29b-41d4-a716-446655440002	EventOwner=$sys	EventTimestamp=2024-01-15T10:32:00Z	EventLocation=TERRA|47.619463|-122.518691	EventLocationSeparator=|	EventType=system log object
```

### Storing Tags
New Tags are stored with the Event [StoreEvent Intent type] or as a batch of Tags [StoreBatchTags Intent type]. Previously stored Tags update using [UpdateBatchTags Intent type]. Tags can be applied to any Event including Linking Events. An indexed tag may be up to 1,000 bytes in length and must be terminated by a null (zero) byte for purposes of storage and retrieval (internally, the zero byte is discarded). Tags have an associated frequency which is a positive, non-zero 64-bit integer. If an existing tag is re-stored with a negative frequency, it is considered inactive and will not be returned in subsequent searches. If a tag is stored with a frequency of zero, the Neural Memory DB service stores it outside of the index just as an attached value. Tags have no specific formatting requirements aside from the null byte termination. Tags may be “owned” by an event, in which case they can be found only via searches where the owner event ID is provided. This permits for “private” sets of tags to be associated with an event. Event ID “$sys” indicates that the tags are associated with the “system”, and are therefore accessible to public searches where no owner is specified.

#### Rules:
- Frequency field an int, is used to track the number of instances of the Tag. 
- Key field: can be any alphanumeric value used to identify the Tag. Examples include: text (e.g., topic:quantum_mechanics), dense or sparse embedding vectors (e.g., [0.23, -0.45, 0.67, ...]), hash (e.g., seq_hash_12847), and pointers (e.g., actor@gateway:EVENTID).
- Value field: can be any value as the Tag value. Examples include: text (e.g., user:john_conversation_2024-01-15), dense or sparse embedding vector (e.g., [0.23, -0.45, 0.67, ...]), hash (e.g., seq_hash_12847), and pointers (e.g., actor@gateway:EVENTID) or binary data. 
- Values are automatically versioned based on the; by default the most recent is returned during retrieval. 
- There is no upper bound on the number of Tags and Event can have; however, the greater the number the slower the storage and retrieval latencies and therefore the maximum number of Tags is a tunable hyperparameter based on the use case and latency requirements. 

#### Example Pod-OS Formats:

##### As StoreEvent Intent:
```
systemLogTagList := []Tag{
		Tag{Frequency: 1, Key: "domain", Value: domainName},
		Tag{Frequency: 1, Key: "log_type", Value: "system"},
		Tag{Frequency: 1, Key: "severity", Value: "INFO"},
		Tag{Frequency: 1, Key: "component", Value: "pod-os-core"},
		Tag{Frequency: 1, Key: "timestamp", Value: message.GetTimestamp()}
	}

Message{
		To:                     "administration@zeroth." + domainName, // The recipient actor@gateway
		From:                   "Pod-OSDashboardNode@zeroth." + domainName, // The sender in the form client@gateway. This field must contain the same client and gateway name used to establish the connection.
		ClientName:             "Pod-OSDashboardNode", // The client name, must be the same name used when establishing the connection. 
		MessageId:              [GUID], // A unique identifier for the message to track the message and conversation flow.
		Intent:                 IntentType.StoreEvent, // The intent of the message to store the event object.
		EventOwner:             [EventID from the Base Event], // The owner of the event object.
		EventUniqueId:          [GUID], // A unique identifier for the event object.
		EventTimestamp:         message.GetTimestamp(), // The timestamp of the event object.
		EventLocation:          "TERRA|47.619463|-122.518691", // The location of the event object; can be geospatial, temporal, or other.
		EventLocationSeparator: "|", // The separator for the event location.
		EventType:              "system log object", // String that describes the type of event object.
		PayloadMimeType:        "text/plain", // The MIME type of the payload data.
		PayloadData:            "System initialization log", // The data of the event object.
		PayloadDataType:        RAW, // The data type of the payload data.
		Tags:                   systemLogTagList, // The tags to be applied to the event object.
	}
```

##### As StoreBatchTags Intent: 
```
Message{
		To:                     "administration@zeroth." + domainName, // The recipient actor@gateway
		From:                   "Pod-OSDashboardNode@zeroth." + domainName, // The sender in the form client@gateway. This field must contain the same client and gateway name used to establish the connection.
		ClientName:             "Pod-OSDashboardNode", // The client name, must be the same name used when establishing the connection.
		MessageId:              [GUID], // A unique identifier for the message to track the message and conversation flow.
		Intent:                 IntentType.StoreBatchTags, // The intent of the message to store the batch of tags.
		EventOwner:             [EventOwnerId], // The owner of the event object.
		EventId:          		[EventId], // EventId must already exist in Neural Memory store
		PayloadMimeType:        "text/plain", // The MIME type of the payload data.
		PayloadData:            [text payload], // The data of the batch of tags.
		PayloadDataType:        RAW, // The data type of the payload data.
	}
```

The text payload contains a series of newline-terminated records, each formatted as frequency=tagvalue. 
For example:
10=the
12=then
100=and

Facets are a variation of tagvalue which enables greater flexibility and improved search. Their pattern is:
frequency=key_name=key_value


##### As UpdateBatchTags Intent: 
```
Message{
		To:                     "administration@zeroth." + domainName, // The recipient actor@gateway
		From:                   "Pod-OSDashboardNode@zeroth." + domainName, // The sender in the form client@gateway. This field must contain the same client and gateway name used to establish the connection.
		ClientName:             "Pod-OSDashboardNode", // The client name, must be the same name used when establishing the connection.
		MessageId:              [GUID], // A unique identifier for the message to track the message and conversation flow.
		Intent:                 IntentType.UpdateBatchTags, // The intent of the message to update the batch of tags.
		EventOwner:             [EventOwnerId], // The owner of the event object.
		EventId:          		[EventId], // EventId must already exist in Neural Memory store
		PayloadMimeType:        "text/plain", // The MIME type of the payload data.
		PayloadData:            [text payload], // The data of the batch of tags.
		PayloadDataType:        RAW, // The data type of the payload data.
	}
```

The text payload contains a series of newline-terminated records, each formatted as frequency=tagvalue.
For example:
10=the
12=then
100=and

Facets are a variation of tagvalue which enables greater flexibility and improved search. Their pattern is:
frequency=key_name=key_value


### Linking Events

Links are created [LinkEvents Intent type] between any two Event Objects. Links are intended to create networks of explicitly associated events, where the strength of an association (float value) can be set. Two strength values are used so that in cases where there is a many-to-many relationship, the link strength can be set differently depending on the direction of traversal. Links are Event Objects and may be treated as such, which means that Tags may be applied to the Links themselves for future use in Retrievals based on Tag values. It also means that Links can be used to create complex data structures and sets of related events including Links to Links.

All links are organized into sets, referred to as “categories” in Pod-OS. The intent of categories is to create groups of related links based on usage or type. There may be multiple links between the same events, so long as the links are in separate categories. A category is simply a name stored as a string of characters with a null (zero) byte terminator. There is no central directory or list of categories, though one can certainly be created within a database if desired.


##### Rules:
- An Event may be a Storage Event or a Link Event; therefore linking between Links or Link to Store Event or any combination is a valid operation. 
- There is no upper bound on the number of Links between Event Objects.
- Category value can be any null-terminated ASCII character string.

##### Example Pod-OS Format:
```
Message{
		To:                     "account@zeroth." + domainName, // The recipient actor@gateway
		From:                   "Pod-OSDashboardNode@zeroth." + domainName, // The sender in the form client@gateway. This field must contain the same client and gateway name used to establish the connection.
		ClientName:             "Pod-OSDashboardNode", // The client name, must be the same name used when establishing the connection.
		MessageId:              [GUID], // A unique identifier for the message to track the message and conversation flow.
		Intent:                 IntentType.LinkEvents, // The intent of the message to link the two event objects.
		EventOwner:             [EventOwnerId], // The owner of the Link event object.
		EventUniqueId:          [GUID], // A unique identifier for the Link event object.
		EventTimestamp:         message.GetTimestamp(), // The timestamp of the Link event object.
		EventLocation:          "TERRA|47.619463|-122.518691", // The location of the Link event object; can be geospatial, temporal, or other.
		EventLocationSeparator: "|", // The separator for the Link event location.
		EventType:              "account link admin account to admin group", // String that describes the type of Link event object.
		PayloadMimeType:        "", // The MIME type of the payload data.
		PayloadData:            nil, // The data of the Link event object.
		Link: Link{
			LinkUniqueIdA:         [EventIdA], // The EventID of the first event object.
			LinkUniqueIdB:         [EventIdB], // The EventID of the second event object.
			LinkStrengthA:         1.0, // The strength of the link from the first event object to the second event object.
			LinkStrengthB:         1.0, // The strength of the link from the second event object to the first event object.
			LinkOwner:             [EventOwnerId or EventOwnerUniqueId], // The creator of the Link event object.
			LinkTimestamp:         message.GetTimestamp(), // The timestamp of the Link event object.
			LinkLocation:          "TERRA|47.619463|-122.518691", // The location of the Link event object; can be geospatial, temporal, or other.
			LinkLocationSeparator: "|", // The separator for the Link event location.
			LinkCategory:			 "Account" // The category of the Link event object.
		},
	}
```
#### Unlink
Unlink two Events [UnlinkEvents Intent type]. 

##### Rules:
- An Event may be a Storage Event or a Link Event; therefore linking between Links or Link to Store Event or any combination is a valid operation. 
- There is no upper bound on the number of Links between Events.

##### Example Pod-OS Format:
```
Message{
		To:                     "account@zeroth." + domainName,
		From:                   "Pod-OSDashboardNode@zeroth." + domainName,
		ClientName:             "Pod-OSDashboardNode",
		Intent:                 IntentType.UnlinkEvents,
		MessageId: 				{{GUID}}
		EventId:             	{{EventId}}, // The EventID of the Link Event
		EventTimestamp:         time.Now().String(),
		EventLocation:          "TERRA|47.619463|-122.518691",
		EventLocationSeparator: "|",
	}
```

### Store Batch Events
Store a batch of Links [StoreBatchEvents Intent type].

##### Rules:
- There is no upper bound on the number of Links between Events.

##### Example Pod-OS Format:
```
Message{
		To:                     "account@zeroth." + domainName,
		From:                   "Pod-OSDashboardNode@zeroth." + domainName,
		ClientName:             "Pod-OSDashboardNode",
		Intent:                 IntentType.StoreBatchEvents,
		MessageId: 				{{GUID}}
		PayloadData:            [text payload], // The data of the batch of links.
		PayloadDataType:        RAW, // The data type of the payload data.
	}
```

The text payload contains a series of newline-terminated records, each formatted as fieldname=value <tab> fieldname=value ... with the following required fields:
- LinkUniqueIdA: The EventID of the first event object.
- LinkUniqueIdB: The EventID of the second event object.
- LinkStrengthA: The strength of the link from the first event object to the second event object.
- LinkStrengthB: The strength of the link from the second event object to the first event object.
- Category: The category of the Link event object.
- EventOwner: The owner of the Link event object.
- EventUniqueId: A unique identifier for the Link event object.
- EventTimestamp: The timestamp of the Link event object.
- EventLocation: The location of the Link event object; can be geospatial, temporal, or other.
- EventLocationSeparator: The separator for the Link event location.
- EventType: String that describes the type of Link event object.

For example:

```
unique_id_a=1234567890	unique_id_b=1234567891	strength_a=1.0	strength_b=1.0	category=Account	owner=$sys	unique_id=1234567890	time=2024-01-15T10:30:00Z	loc=TERRA|47.619463|-122.518691	loc_delim=|	type=account link admin account to admin group
unique_id_a=1234567890	unique_id_b=1234567892	strength_a=1.0	strength_b=1.0	category=Account	owner=$sys	unique_id=1234567890	time=2024-01-15T10:30:00Z	loc=TERRA|47.619463|-122.518691	loc_delim=|	type=account link admin account to admin group
unique_id_a=1234567890	unique_id_b=1234567893	strength_a=1.0	strength_b=1.0	category=Account	owner=$sys	unique_id=1234567890	time=2024-01-15T10:30:00Z	loc=TERRA|47.619463|-122.518691	loc_delim=|	type=account link admin account to admin group
```
