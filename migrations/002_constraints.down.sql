ALTER TABLE order_items
    DROP CONSTRAINT order_items_name_nonempty,
    DROP CONSTRAINT order_items_quantity_positive,
    DROP CONSTRAINT order_items_price_nonnegative;

ALTER TABLE orders
    DROP CONSTRAINT orders_email_nonempty,
    DROP CONSTRAINT orders_status_valid,
    DROP CONSTRAINT orders_total_nonnegative;

ALTER TABLE products
    DROP CONSTRAINT products_sku_nonempty,
    DROP CONSTRAINT products_name_nonempty,
    DROP CONSTRAINT products_price_nonnegative,
    DROP CONSTRAINT products_stock_nonnegative;