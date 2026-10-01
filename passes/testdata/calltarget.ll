; calltarget.ll — direct call targets. lli baseline: main = twice(21) = 42.
define i32 @helper(i32 %v) {
entry:
  ret i32 %v
}

define i32 @twice(i32 %v) {
entry:
  %r = call i32 @helper(i32 %v)
  %d = mul nsw i32 %r, 2
  ret i32 %d
}

define i32 @main() {
entry:
  %r = call i32 @twice(i32 21)
  ret i32 %r
}
