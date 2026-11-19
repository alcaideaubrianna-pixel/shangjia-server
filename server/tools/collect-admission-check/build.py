#!/usr/bin/env python3
"""Build and run admission regression against actual package via a temporary Go overlay.
No go test or application bootstrap. Then runs disposable local PostgreSQL checks.
"""
import json
from pathlib import Path
import subprocess
import sys
import tempfile

server = Path(__file__).resolve().parents[2]
package = server / 'addons/youban_publish/logic/sys'
source = r'''package sys
import (
 "context"
 "fmt"
 "sync"
 "github.com/gogf/gf/v2/database/gdb"
)
func RunCollectAdmissionRegression() {
 display := collectMediaItem{Type:"photo", Purpose:"display", FileUrl:"https://fixture.invalid/display.jpg"}
 verify := collectMediaItem{Type:"video", Purpose:"verify", FileUrl:"https://fixture.invalid/verify.mp4"}
 noFile := verify; noFile.FileUrl=""; noFile.FileId="bot-file-only"
 wrongPurpose := verify; wrongPurpose.Purpose="display"
 wrongType := display; wrongType.Purpose="verify"
 cases := []struct{name string; media []collectMediaItem; count int; allowed bool}{
  {"incident20904504_display_only", []collectMediaItem{display}, 1, false},
  {"incident20906683_duplicate_display", []collectMediaItem{display,display}, 2, false},
  {"incident20906722_video_persisted", []collectMediaItem{display,verify}, 2, true},
  {"verify_file_id_not_downloaded", []collectMediaItem{display,noFile}, 2, false},
  {"display_video_not_verification", []collectMediaItem{display,wrongPurpose}, 2, false},
  {"verification_photo_not_video", []collectMediaItem{display,wrongType}, 2, false},
  {"incomplete_snapshot", []collectMediaItem{display,verify}, 3, false},
  {"empty_snapshot", nil, 0, false},
 }
 for _, c := range cases {
  content := &collectContentResult{Media:c.media, MediaCount:c.count}
  if got := validateCollectMaterialMedia(content)==nil; got!=c.allowed { panic(c.name) }
  fmt.Println("PASS", c.name)
 }
 // Direct commit must reject before metadata, transaction or any database access.
 s := &sSysPublish{}
 for _,closed:=range []bool{false,true} {
  settled,err:=s.collectVerifyEventsSettled(context.Background(),gdb.Record{},closed)
  if err!=nil || settled {panic("empty_pair_must_not_settle_even_when_window_closed")}
 }
 fmt.Println("PASS missing_pair_never_settles_even_when_window_closed")
 _,err := s.commitCollectPreparedProfile(context.Background(), gdb.Record{}, &collectPreparedMaterial{
  Content:&collectContentResult{}, Media:[]collectPreparedMedia{{MediaType:"image", Purpose:"display"}},
 }, gdb.Record{}, "fixture")
 if !isCollectProcessRetryError(err) { panic("direct_commit_did_not_retry") }
 fmt.Println("PASS direct_commit_rejects_without_db")
 publishCases := []struct{name string; event int64; media []*telegramMediaItem; allowed bool}{
  {"historical_collect_job_missing_verify",20904504,nil,false},
  {"historical_collect_job_verify_photo",20904504,[]*telegramMediaItem{{MediaType:"image"}},false},
  {"historical_collect_job_nil_verify",20904504,[]*telegramMediaItem{nil},false},
  {"collected_job_has_verification_video",20904504,[]*telegramMediaItem{{MediaType:"video"}},true},
  {"manual_display_only_contract_preserved",0,nil,true},
 }
 for _, c := range publishCases {
  if telegramCollectedVerificationReady(telegramJobRecord{CollectEventId:c.event},c.media)!=c.allowed {panic(c.name)}
  fmt.Println("PASS",c.name)
 }
 var wg sync.WaitGroup
 for worker:=0; worker<32; worker++ {
  wg.Add(1); go func(){ defer wg.Done(); for i:=0;i<1000;i++ {
   for _,c:=range cases { if got:=validateCollectMaterialMedia(&collectContentResult{Media:c.media,MediaCount:c.count})==nil;got!=c.allowed {panic(c.name)} }
  }}()
 }
 wg.Wait()
 fmt.Println("PASS concurrent_32_workers_256000_checks")
}
'''
with tempfile.TemporaryDirectory(prefix='collect-admission-') as temp:
    temp = Path(temp)
    helper = temp / 'regression.go'
    helper.write_text(source)
    overlay = temp / 'overlay.json'
    overlay.write_text(json.dumps({'Replace': {str(package / 'zz_collect_admission_regression.go'): str(helper)}}))
    main = temp / 'main.go'
    main.write_text('package main\nimport s "hotgo/addons/youban_publish/logic/sys"\nfunc main(){s.RunCollectAdmissionRegression()}\n')
    binary = temp / 'check'
    subprocess.run(['go','build','-overlay',str(overlay),'-o',str(binary),str(main)],cwd=server,check=True)
    subprocess.run([str(binary)],cwd=temp,check=True)

# The PG runner owns a disposable loopback cluster and always tears it down.
subprocess.run([sys.executable, str(Path(__file__).with_name("postgres.py"))], cwd=server, check=True)
