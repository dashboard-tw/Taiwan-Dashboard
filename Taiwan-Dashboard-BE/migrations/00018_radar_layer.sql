-- +goose Up

ALTER TABLE app.component_map_layer DROP CONSTRAINT component_map_layer_layer_type_check;
ALTER TABLE app.component_map_layer ADD CONSTRAINT component_map_layer_layer_type_check
    CHECK (layer_type IN ('circle', 'symbol', 'fill', 'line', 'heatmap', 'raster'));

ALTER TABLE data.weather_image DROP CONSTRAINT weather_image_product_check;
ALTER TABLE data.weather_image ADD CONSTRAINT weather_image_product_check
    CHECK (product IN ('radar', 'satellite', 'radar_layer'));
ALTER TABLE data.weather_image
    ADD COLUMN west  numeric,                       -- 影像四邊的經緯度（WGS 84），只有可以疊圖的產品才有
    ADD COLUMN south numeric,
    ADD COLUMN east  numeric,
    ADD COLUMN north numeric;

-- +goose Down
ALTER TABLE data.weather_image DROP COLUMN west, DROP COLUMN south, DROP COLUMN east, DROP COLUMN north;
DELETE FROM data.weather_image WHERE product = 'radar_layer';
ALTER TABLE data.weather_image DROP CONSTRAINT weather_image_product_check;
ALTER TABLE data.weather_image ADD CONSTRAINT weather_image_product_check
    CHECK (product IN ('radar', 'satellite'));
DELETE FROM app.component_map_layer WHERE layer_type = 'raster';
ALTER TABLE app.component_map_layer DROP CONSTRAINT component_map_layer_layer_type_check;
ALTER TABLE app.component_map_layer ADD CONSTRAINT component_map_layer_layer_type_check
    CHECK (layer_type IN ('circle', 'symbol', 'fill', 'line', 'heatmap'));
