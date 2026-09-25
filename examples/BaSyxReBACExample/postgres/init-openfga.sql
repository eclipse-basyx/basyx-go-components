-- OpenFGA keeps its tuples in a separate database with its own user.
CREATE ROLE openfga WITH LOGIN PASSWORD 'openfga';
CREATE DATABASE openfga OWNER openfga;
