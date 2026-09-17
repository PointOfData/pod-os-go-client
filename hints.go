package podos

// ReplyRoutingTimeoutHint is appended to response-timeout errors after a successful
// GatewayId handshake. Handshake success plus later silence usually means the gateway
// could not route the reply to the registered ClientName/From — not missing Auth0 credentials.
const ReplyRoutingTimeoutHint = " If GatewayId succeeded, the gateway may not have routed the reply: use a unique ClientName and From = ClientName@<dialed-gateway-FQN>. This is not an authentication failure."
