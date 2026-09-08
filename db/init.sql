CREATE DATABASE IF NOT EXISTS twopc;
USE twopc;

CREATE TABLE orders (
    id CHAR(36) PRIMARY KEY,
    sku VARCHAR(100) NOT NULL,
    quantity INT UNSIGNED NOT NULL,
    delivery_address VARCHAR(255) NOT NULL,
    status ENUM('PREPARING', 'COMMITTED', 'ABORTED') NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE inventory (
    sku VARCHAR(100) PRIMARY KEY,
    available_quantity INT UNSIGNED NOT NULL
);

CREATE TABLE store_reservations (
    order_id CHAR(36) PRIMARY KEY,
    sku VARCHAR(100) NOT NULL,
    quantity INT UNSIGNED NOT NULL,
    status ENUM('PREPARED', 'COMMITTED', 'ABORTED') NOT NULL
);

CREATE TABLE delivery_reservations (
    slot_id INT PRIMARY KEY,
    order_id CHAR(36) UNIQUE NULL,
    delivery_address VARCHAR(255) NULL,
    status ENUM('AVAILABLE', 'PREPARED', 'COMMITTED') NOT NULL
);

INSERT INTO inventory (sku, available_quantity) VALUES ('coffee-mug', 10);
INSERT INTO delivery_reservations (slot_id, status) VALUES (1, 'AVAILABLE'), (2, 'AVAILABLE');
