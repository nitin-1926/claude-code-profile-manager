// Ref-counted body scroll lock shared by every overlay. Each overlay used to
// save and restore document.body.style.overflow on its own, so two open at once
// (mobile nav, then Cmd+K) restored each other's "hidden" on close and left the
// page unscrollable. The first lock saves the value; only the last unlock
// restores it, whatever order the overlays close in.
let locks = 0;
let saved = "";

export function lockBodyScroll(): () => void {
  if (locks++ === 0) {
    saved = document.body.style.overflow;
    document.body.style.overflow = "hidden";
  }
  let released = false;
  return () => {
    if (released) return;
    released = true;
    if (--locks === 0) document.body.style.overflow = saved;
  };
}
