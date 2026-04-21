# Todo: Replace this with prometheus timing metrics later
from core.settings import settings


def log_timing(
    t_read: float,
    t_infer: float,
    t_enq: float,
    t_enc: float,
    n_frames: int,
    n_batches: int,
) -> None:
    loop = t_read + t_infer + t_enq
    print(
        f"\n=== per-phase breakdown ({n_frames} frames, batch={settings.BATCH_SIZE}) ==="
    )
    for label, val in [
        ("decode pipe read", t_read),
        ("batch infer     ", t_infer),
        ("encode queue put", t_enq),
    ]:
        print(
            f"  {label}  {val:6.2f}s  {100 * val / loop:5.1f}%  ({1000 * val / n_frames:5.1f}ms/frame)"
        )
    print(f"  {'loop total     '}  {loop:6.2f}s")
    print(f"  encode thread wait  {t_enc:6.2f}s")
    print(
        f"  batches: {n_batches} ({settings.BATCH_SIZE} frames each, last may be partial)"
    )
    print()
