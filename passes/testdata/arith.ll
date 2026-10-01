; arith.ll — integer arithmetic + a phi-based loop.
; lli semantic baseline: main returns add(3,4) + loop(4) = 7 + 6 = 13.
define i32 @add(i32 %a, i32 %b) {
entry:
  %r = add nsw i32 %a, %b
  ret i32 %r
}

define i32 @loop(i32 %n) {
entry:
  br label %cond
cond:
  %i = phi i32 [ 0, %entry ], [ %next, %body ]
  %acc = phi i32 [ 0, %entry ], [ %sum, %body ]
  %done = icmp sge i32 %i, %n
  br i1 %done, label %exit, label %body
body:
  %sum = add nsw i32 %acc, %i
  %next = add nsw i32 %i, 1
  br label %cond
exit:
  ret i32 %acc
}

define i32 @main() {
entry:
  %x = call i32 @add(i32 3, i32 4)
  %y = call i32 @loop(i32 4)
  %r = add nsw i32 %x, %y
  ret i32 %r
}
