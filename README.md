# Go two-phase commit: order placement

This is a deliberately small, local simulation of 2PC for placing an order:

- `OrderService` is the coordinator.
- `StoreService` reserves stock.
- `DeliveryService` reserves delivery capacity.

It demonstrates three outcomes:

- `order-100`: StoreService and DeliveryService durably vote `YES`, so OrderService records and broadcasts `COMMIT`.
- `order-101`: DeliveryService votes `NO`, so OrderService records and broadcasts `ABORT`.
- `order-102`: DeliveryService takes longer than OrderService's prepare timeout, so it records and broadcasts `ABORT`.

Run it with:

```sh
go run .
```

OrderService creates a `context.WithTimeout` for every prepare request. A missed deadline is treated as a failed vote and aborts the order. The service logs use files to model the write-ahead records that make recovery possible. The program removes its temporary logs at exit. A production version would replace direct method calls with HTTP or gRPC, preserve logs across restarts, retry decision delivery, and implement participant recovery.
