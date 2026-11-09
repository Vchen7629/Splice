"""
basicsr (a realesrgan dependency) unconditionally imports
torchvision.transforms.functional_tensor at import time for training-only dataset
code we never use. That module was removed in torchvision>=0.17 (moved to
torchvision.transforms.functional), so shim it back before realesrgan is imported.
"""

import sys
from types import ModuleType

if "torchvision.transforms.functional_tensor" not in sys.modules:
    from torchvision.transforms import functional as _tv_functional

    _shim = ModuleType("torchvision.transforms.functional_tensor")
    setattr(_shim, "rgb_to_grayscale", _tv_functional.rgb_to_grayscale)
    sys.modules["torchvision.transforms.functional_tensor"] = _shim
