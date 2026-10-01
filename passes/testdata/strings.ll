; strings.ll — private string constants whose only users are instructions
; (P2.2 string-encryption fixtures). lli baseline: prints "hello gp 42!" and
; "bye!", exits 0.
@.str = private unnamed_addr constant [14 x i8] c"hello gp %d!\0A\00", align 1
@.str.1 = private unnamed_addr constant [6 x i8] c"bye!\0A\00", align 1

declare i32 @printf(ptr, ...)

define i32 @main() {
entry:
  %c1 = call i32 (ptr, ...) @printf(ptr @.str, i32 42)
  %c2 = call i32 (ptr, ...) @printf(ptr @.str.1)
  ret i32 0
}
