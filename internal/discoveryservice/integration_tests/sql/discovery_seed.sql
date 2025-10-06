-- Seed AAS identifiers with fixed IDs for predictable paging (id asc)
INSERT INTO aas_identifier (id, aasId) VALUES
  (1, 'urn:aas:one'),
  (2, 'urn:aas:two'),
  (3, 'urn:aas:three')
ON CONFLICT (aasId) DO NOTHING;

-- Ensure asset_link is clean for these refs
DELETE FROM asset_link WHERE aasRef IN (1,2,3);

-- AAS 1 has (name1,value1) + (name2,value2) → matches filter
INSERT INTO asset_link (name, value, aasRef) VALUES
  ('name1', 'value1', 1),
  ('name2', 'value2', 1);

-- AAS 2 has only (name1,value1) → should NOT match AND(name1=value1, name2=value2)
INSERT INTO asset_link (name, value, aasRef) VALUES
  ('name1', 'value1', 2);

-- AAS 3 has (name1,value1) + (name2,value2) (+ extra) → matches filter
INSERT INTO asset_link (name, value, aasRef) VALUES
  ('name1', 'value1', 3),
  ('name2', 'value2', 3),
  ('name3', 'valueX', 3);

-- optional: keep sequences ahead of our explicit IDs (not strictly required)
-- SELECT setval('aas_identifier_id_seq', GREATEST((SELECT COALESCE(MAX(id),0) FROM aas_identifier), 3) + 1, true);
-- SELECT setval('asset_link_id_seq', GREATEST((SELECT COALESCE(MAX(id),0) FROM asset_link), 5) + 1, true);
