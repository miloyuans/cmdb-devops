#!/usr/bin/env sh
set -eu
ROOT=${1:-.}
TMP=${TMPDIR:-/tmp}/cmdb_parse_check.go
cat > "$TMP" <<'GO'
package main
import (
  "fmt"
  "go/parser"
  "go/token"
  "os"
  "path/filepath"
)
func main(){
  fs:=token.NewFileSet(); ok:=true
  filepath.Walk(os.Args[1], func(path string, info os.FileInfo, err error) error{
    if err!=nil || info==nil || info.IsDir() || filepath.Ext(path)!=".go" { return nil }
    if _,err:=parser.ParseFile(fs,path,nil,parser.AllErrors); err!=nil { fmt.Println(err); ok=false }
    return nil
  })
  if !ok { os.Exit(1) }
  fmt.Println("go syntax check ok")
}
GO
go run "$TMP" "$ROOT"
