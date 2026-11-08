ALTER TABLE products
    ADD CONSTRAINT products_sku_nonempty
        CHECK (length(trim(sku)) > 0),
    ADD CONSTRAINT products_name_nonempty
        CHECK (length(trim(name)) > 0),
    ADD CONSTRAINT products_price_nonnegative
        CHECK (price_cents >= 0),
    ADD CONSTRAINT products_stock_nonnegative
        CHECK (stock >= 0);

ALTER TABLE orders
    ADD CONSTRAINT orders_email_nonempty
        CHECK (length(trim(customer_email)) > 0),
    ADD CONSTRAINT orders_status_valid
        CHECK (status IN ('created', 'cancelled')),
    ADD CONSTRAINT orders_total_nonnegative
        CHECK (total_cents >= 0);

ALTER TABLE order_items
    ADD CONSTRAINT order_items_name_nonempty
        CHECK (length(trim(product_name)) > 0),
    ADD CONSTRAINT order_items_quantity_positive
        CHECK (quantity > 0),
    ADD CONSTRAINT order_items_price_nonnegative
        CHECK (unit_price_cents >= 0);