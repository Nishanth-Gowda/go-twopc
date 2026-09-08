# MySQL-backed 2PC order demo

`OrderService` coordinates a two-phase commit across `StoreService` and `DeliveryService`.

## Start MySQL

```sh
docker compose up -d
```

The database is seeded with ten `coffee-mug` items and two delivery slots.

## Run the services

Open three terminals in this directory:

```sh
go run ./cmd/store-service
go run ./cmd/delivery-service
go run ./cmd/order-service
```

All services default to `root:root@tcp(127.0.0.1:3306)/twopc?parseTime=true`. Set `MYSQL_DSN` to use another database connection.

## Live dashboard

In a fourth terminal, run:

```sh
cd dashboard
npm run dev
```

Open [http://localhost:3000](http://localhost:3000). The dashboard refreshes the order ledger, inventory, and delivery slots every second. Its order form also includes a timeout toggle for the delivery-delay demonstration.

## Try 2PC

Successful order:

```sh
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"sku":"coffee-mug","quantity":1,"delivery_address":"42 Main Street"}'
```

Abort because inventory is insufficient:

```sh
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -d '{"sku":"coffee-mug","quantity":99,"delivery_address":"42 Main Street"}'
```

Abort because DeliveryService misses the two-second prepare deadline:

```sh
curl -X POST http://localhost:8080/orders \
  -H 'Content-Type: application/json' \
  -H 'X-Delivery-Prepare-Delay-Ms: 3000' \
  -d '{"sku":"coffee-mug","quantity":1,"delivery_address":"42 Main Street"}'
```

Use `GET /orders/{id}` to read the persisted order status. StoreService (`:8081`) and DeliveryService (`:8082`) also expose `POST /prepare`, `POST /commit`, and `POST /abort` for observing the internal protocol.
