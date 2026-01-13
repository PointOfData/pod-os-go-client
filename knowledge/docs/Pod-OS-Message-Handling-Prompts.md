
##########################
The Dashboard Client communicates with Gateway Actors and Actors via podos/SendMessage() and MessageSendResult()(consider if this should be renamed to MessageResponse() or similar). As you can see, SendMessage() takes a Message, serializes the message into a byte message, and sends to a Gateway Actor's socket connection. The Gateway uses the To and From addressing to route the message. Messages are sent and received much like e-mail. 

Message structure: Messages are composed of two address specifications, a header, a numeric message type, and an optional data payload which may be up to 2 gigabytes in size. The address specifications are ASCIIZ strings, as is the header. The message type is a standard 32-bit signed integer, and the data payload is an unformatted buffer. The payload size specification is a standard 32-bit signed integer.

Connection Event Sequence: When connecting to a Gateway Actor, a socket connection is first established at which point the Gateway Actor is aware that there is a client, but has no other information about the connection. The Gateway Actor assigns the connection a temporary internal name. Following the connection, an identifier message is sent by the connecting client which identifies the connection point so that message traffic can be routed appropriately. 

The ID message is required before any other messages will be recognized. Until the ID message is received, all messages received from the new client will be ignored. However, shutdown or forced disconnect messages may be sent to the new service. Once an ID is established, messages can be addressed and delivered to the specified Actor@Gateway Actor.

Message uses one of two states: a. the Gateway is streaming responses for asynchronous message ("STREAM ON"), or b. synchronous message mode where the Client requests message one at a time from a mailbox queue ("STREAM OFF"). Default state is "STREAM OFF". The Pod-OS Dashboard client, for responsiveness uses STREAM ON by default as you can see from the connection sequence. 

There are two uses cases to support:
1./ Client (for example the Pod-OS Dashboard) uses SendMessage() to send a message and MessageSendResult() to process the response into JSON to manage Actors. 
2./ Optionally, customers may use SocketIO Events vended by the Dashboard software acting as a proxy client to exchange JSON objects and stream binary payload attachments. SocketIO Events are not provided by this package; check with the Dashboard software for details.