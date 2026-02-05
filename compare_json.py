import json
import sys

with open('internal/submodelrepository/integration_tests/logs/STEP_335.log', 'r') as f:
    content = f.read()

parts = content.split('Actual:')
expected_str = parts[0].replace('JSON mismatch:', '').strip()
actual_str = parts[1].strip()

expected = json.loads(expected_str)
actual = json.loads(actual_str)

def find_value_diffs(exp, act, path=''):
    diffs = []
    if isinstance(exp, dict) and isinstance(act, dict):
        all_keys = set(exp.keys()) | set(act.keys())
        for key in all_keys:
            new_path = f'{path}.{key}' if path else key
            if key not in exp:
                diffs.append(f'EXTRA: {new_path}')
            elif key not in act:
                diffs.append(f'MISSING: {new_path}')
            else:
                diffs.extend(find_value_diffs(exp[key], act[key], new_path))
    elif isinstance(exp, list) and isinstance(act, list):
        if len(exp) != len(act):
            diffs.append(f'LENGTH: {path} expected={len(exp)} actual={len(act)}')
    elif exp != act:
        diffs.append(f'VALUE: {path} expected="{exp}" actual="{act}"')
    return diffs

diffs = find_value_diffs(expected, actual)
for d in diffs:
    print(d)
