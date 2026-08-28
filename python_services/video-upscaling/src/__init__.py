import sys
from pathlib import Path
from types import ModuleType

# for absolute imports
_src_dir = str(Path(__file__).parent)
if _src_dir not in sys.path:
    sys.path.insert(0, _src_dir)

# basicsr (a realesrgan dependency) unconditionally imports
# torchvision.transforms.functional_tensor at package-import time for training-only
# dataset code we never use. That module was removed in torchvision>=0.17 (moved to
# torchvision.transforms.functional), which crashes the import outright. Shim it back
# before anything in this package can trigger that import chain.
if "torchvision.transforms.functional_tensor" not in sys.modules:
    from torchvision.transforms import functional as _tv_functional

    _shim = ModuleType("torchvision.transforms.functional_tensor")
    setattr(_shim, "rgb_to_grayscale", _tv_functional.rgb_to_grayscale)
    sys.modules["torchvision.transforms.functional_tensor"] = _shim
