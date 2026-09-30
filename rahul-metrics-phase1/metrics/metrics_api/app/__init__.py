"""Metrics API. Imports DTOs from minidd_contracts.models (Darsan's package).

Until contracts v1.0.0 is pip-installable, fall back to the verbatim subset in ../_stub_contracts.
Delete the stub directory (and this block) once `pip install -e contracts/python` works.
"""
import logging
import os
import sys

try:
    import minidd_contracts  # noqa: F401
except ImportError:
    _stub = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "_stub_contracts")
    if os.path.isdir(_stub):
        sys.path.insert(0, _stub)
        logging.getLogger("metrics-api").warning("minidd_contracts not installed; using local stub subset")
