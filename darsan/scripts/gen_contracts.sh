#!/bin/sh
# Generates contracts/jsonschema/*.json from the pydantic models (TS types: run `npx json-schema-to-typescript` on these).
set -e
mkdir -p contracts/jsonschema
python - <<'PY'
import json, inspect
from minidd_contracts import models as m
for n, c in inspect.getmembers(m, inspect.isclass):
    if issubclass(c, m.BaseModel) and c.__module__ == m.__name__ and not c.__pydantic_generic_metadata__["parameters"]:
        json.dump(c.model_json_schema(), open(f"contracts/jsonschema/{n}.json", "w"), indent=1)
PY
