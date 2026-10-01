; branchy.ll — branching shapes for cf_flatten / dump-cfg fixtures.
; lli baseline: main = classify(1) = 20. classify itself has a switch
; terminator, so cf_flatten must skip it and still flatten noise().
define void @noise(i32 %x) {
entry:
  %c = icmp sgt i32 %x, 10
  br i1 %c, label %then, label %else
then:
  %big = mul nsw i32 %x, 2
  br label %join
else:
  %small = sub nsw i32 %x, 1
  br label %join
join:
  ret void
}

define i32 @classify(i32 %v) {
entry:
  switch i32 %v, label %default [
    i32 0, label %zero
    i32 1, label %one
  ]
zero:
  ret i32 10
one:
  ret i32 20
default:
  ret i32 30
}

define i32 @main() {
entry:
  call void @noise(i32 42)
  %r = call i32 @classify(i32 1)
  ret i32 %r
}
