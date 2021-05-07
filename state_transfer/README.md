# Introduction
Each state transfer consists of 3 components.
- A transfer utility such as rclone or rsync
- A transport used to secure traffic between clusters
- An endpoint type for exposing the server/source side for transfer

# Trasfer
Currently [rsync](https://rsync.samba.org/) and [rclone](https://rclone.org/) are available.

# Transport
Two transports are available.

## Null
The null transport is intended for any potential future options that provide their own encryption between client and server. It may also be useful for troubleshooting, but shouldn't be used with unencrypted protocols with sensitive data.

## Stunnel
[Stunnel](https://www.stunnel.org/) is a proxy that provides TLS encryption without having to change existing clients and servers.

# Endpoint
## Route
Routes are available and commonly used in openshift clusters

## Load Balancer
An alternative to routes that will work with other Kubernetes implementations

# Compatibility Matrix
<table>
    <thead>
        <tr>
            <th>Transfer</th>
            <th>Transport</th>
            <th>Endpoint</th>
            <th>State</th>
        </tr>
    </thead>
    <tbody>
        <tr>
            <td rowspan=4>rclone</td>
            <td rowspan=2>null</td>
            <td>load balancer</td>
            <td>functional but inseure</td>
        </tr>
        <tr>
            <td>route</td>
            <td>functional but insecure</td>
        </tr>
        <tr>
            <td rowspan=2>stunnel</td>
            <td>load balancer</td>
            <td>functional</td>
        </tr>
        <tr>
            <td>route</td>
            <td>functional</td>
        </tr>
        <tr>
            <td rowspan=4>rsync</td>
            <td rowspan=2>null</td>
            <td>load balancer</td>
            <td>functional but inseure</td>
        </tr>
        <tr>
            <td>route</td>
            <td>nonfunctional</td>
        </tr>
        <tr>
            <td rowspan=2>stunnel</td>
            <td>load balancer</td>
            <td>functional</td>
        </tr>
        <tr>
            <td>route</td>
            <td>functional</td>
        </tr>
    </tbody>
</table>

# TODO
- Implement check for clients / servers to ensure pods come up and in the case of servers are ready to send data.
- Implement check for load balancers to resolve
- Look into nodePort as an alternative to LB and route
- Implement functions to tear down servers and clients when the transfer is complete

The lack of progress checks do not cripple functionality, but the client side may error serveral times while servers come up and hostnames become resolvable, which isn't very pretty.
