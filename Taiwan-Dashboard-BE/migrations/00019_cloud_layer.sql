-- +goose Up
ALTER TABLE data.weather_image DROP CONSTRAINT weather_image_product_check;
ALTER TABLE data.weather_image ADD CONSTRAINT weather_image_product_check
    CHECK (product IN ('radar', 'satellite', 'radar_layer', 'cloud_layer'));

-- +goose Down
DELETE FROM data.weather_image WHERE product = 'cloud_layer';
ALTER TABLE data.weather_image DROP CONSTRAINT weather_image_product_check;
ALTER TABLE data.weather_image ADD CONSTRAINT weather_image_product_check
    CHECK (product IN ('radar', 'satellite', 'radar_layer'));
