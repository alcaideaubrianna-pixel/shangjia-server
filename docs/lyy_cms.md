关于上架插件, 我需要实现一个旧系统到当前系统的迁移功能, 主要是迁移所有的资料,  而且旧的系统非常多, 所以这个也需要实现成各种插件来源的插件拓展, 因为后续有非常多的来源, 到时候我们可以直接通过后台导
  入数据, 2.导入数据做成后台持续任务, 因为有两点, 1.采集笔记数据, 2.COS 迁移, 从原来的服务器迁移到 COS 上, 3.采集数据对应的 telegram 消息 ID,   并且这个进度对应的账号也可以看到, 后台也可以看到, 并
  且可以配置 IP代理池, 支持随机 IP 拉取图片视频静态资源,  这个是开关, 还有测试的时候, 支持采集数量, 比如说采集 100 个, 50 个之类的, Server web 后台也需要增加笔记列, 支持批量查找或者删除, 笔记对应所属租户
  和账号,  先告诉你原来的数据源, 
  1.登录, https://jiugui.yyby521.xyz/user/login (网络请求域名可以每个都不一样) 请求 POSt body
  _csrf_token=GHvuSkXWLqIR8fCcMM7OXJ3kPfz_tIYX_neOcxXKaJs&username=jiuguibaoyang&password=Lzc890211, 
  登录成功的响应 HTTP/1.1 303 See Other
Server: nginx/1.18.0 (Ubuntu)
Date: Mon, 06 Jul 2026 08:56:04 GMT
Content-Length: 0
Connection: keep-alive
location: https://jiugui.yyby521.xyz/user/profile
x-frame-options: DENY
x-content-type-options: nosniff
x-xss-protection: 1; mode=block
referrer-policy: strict-origin-when-cross-origin
content-security-policy: default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:;
set-cookie: session=eyJfZmxhc2hlcyI6IFtbInN1Y2Nlc3MiLCAiXHU3NjdiXHU1ZjU1XHU2MjEwXHU1MjlmXHVmZjAxIl1dLCAiX2NzcmZfdG9rZW4iOiAiR0h2dVNrWFdMcUlSOGZDY01NN09YSjNrUGZ6X3RJWVhfbmVPY3hYS2FKcyIsICJ1c2VyX211c3RfY2hhbmdlX3B3ZCI6IGZhbHNlLCAidXNlcl9pZCI6IDEsICJ1c2VyX3VzZXJuYW1lIjogImppdWd1aWJhb3lhbmciLCAidXNlcl9uaWNrbmFtZSI6ICJcdTkxNTJcdTliM2NiYW9cdTUxN2IifQ==.akttpA.IUrP50LsuIuM2npddcfCzGbnHhw; path=/; Max-Age=2592000; httponly; samesite=lax; secure
Strict-Transport-Security: max-age=31536000; includeSubDomains


  2.登录成功返回 set-cookie 进行存储,
  3.调用接口 https://jiugui.yyby521.xyz/user/contents?per_page=12&page=1
   解析出来
   <div class="grid-card">
        <!-- 缩略图 -->
        <div class="thumb-wrap">
            
                
                    <img src="/uploads/20260705154133667835_cb819b14_photo_A3kAAzwE_processed.jpg" alt="" loading="lazy" onerror="this.style.display='none';this.nextElementSibling.style.display='flex'">
                    <div class="thumb-placeholder" style="display:none;"><span class="ic"><svg viewBox="0 0 24 24"><rect x="3" y="3" width="18" height="18" rx="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><polyline points="21 15 16 10 5 21"></polyline></svg></span></div>
                
            

            <!-- 类型角标 -->
            
                <span class="type-badge" style="background:linear-gradient(135deg,#fa709a,#fee140);color:#333;"><span class="ic"><svg viewBox="0 0 24 24"><rect x="2" y="2" width="20" height="20" rx="2.18"></rect><line x1="7" y1="2" x2="7" y2="22"></line><line x1="17" y1="2" x2="17" y2="22"></line><line x1="2" y1="12" x2="22" y2="12"></line><line x1="2" y1="7" x2="7" y2="7"></line><line x1="2" y1="17" x2="7" y2="17"></line><line x1="17" y1="17" x2="22" y2="17"></line><line x1="17" y1="7" x2="22" y2="7"></line></svg></span> 混合</span>
            

            <!-- 状态角标 -->
            
                <span class="status-badge" style="background:#e8f5e9;color:#2e7d32;"><span class="ic"><svg viewBox="0 0 24 24"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg></span> 已上架</span>
            
        </div>

        <!-- 卡片信息 -->
        <div class="card-body">
            <div class="card-title" title="酒鬼bao养353">酒鬼bao养353</div>
            <div class="card-meta">07-05 15:41 · 7 个文件</div>

            <!-- 五个按钮 -->
            <div class="card-btns">
                
                    <form method="post" action="https://jiugui.yyby521.xyz/user/content/unpublish/358" style="display:contents;" onsubmit="return confirm('确定要下架《酒鬼bao养353》吗？');">
                        <input type="hidden" name="_csrf_token" value="GHvuSkXWLqIR8fCcMM7OXJ3kPfz_tIYX_neOcxXKaJs">
                        <button type="submit" class="btn-status-pub"><span class="ic"><svg viewBox="0 0 24 24"><line x1="12" y1="5" x2="12" y2="19"></line><polyline points="19 12 12 19 5 12"></polyline></svg></span> 下架</button>
                    </form>
                
                <a href="https://jiugui.yyby521.xyz/user/content/view/358" class="btn-view"><span class="ic"><svg viewBox="0 0 24 24"><path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"></path><circle cx="12" cy="12" r="3"></circle></svg></span> 查看</a>
                <a href="https://jiugui.yyby521.xyz/user/content/edit/358" class="btn-edit"><span class="ic"><svg viewBox="0 0 24 24"><path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"></path><path d="M18.5 2.5a2.12 2.12 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z"></path></svg></span> 编辑</a>
                <form method="post" action="https://jiugui.yyby521.xyz/user/content/delete/358" style="display:contents;" onsubmit="return confirm('确定要删除《酒鬼bao养353》吗？所有相关文件也会被删除。');">
                    <input type="hidden" name="_csrf_token" value="GHvuSkXWLqIR8fCcMM7OXJ3kPfz_tIYX_neOcxXKaJs">
                    <button type="submit" class="btn-del"><span class="ic"><svg viewBox="0 0 24 24"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg></span> 删除</button>
                </form>
                <a href="https://jiugui.yyby521.xyz/user/content/logs/358" class="btn-log" style="grid-column:1/-1;"><span class="ic"><svg viewBox="0 0 24 24"><line x1="8" y1="6" x2="21" y2="6"></line><line x1="8" y1="12" x2="21" y2="12"></line><line x1="8" y1="18" x2="21" y2="18"></line><line x1="3" y1="6" x2="3.01" y2="6"></line><line x1="3" y1="12" x2="3.01" y2="12"></line><line x1="3" y1="18" x2="3.01" y2="18"></line></svg></span> 发送记录</a>
            </div>
        </div>
    </div>

    然后读取分页

    <span style="background:#fff;padding:8px 16px;border-radius:6px;font-size:14px;color:#666;box-shadow:0 1px 3px rgba(0,0,0,0.1);">
        第 <strong style="color:#2f55e0;">1</strong> / 29 页（共 344 条）
    </span>

    先采集第一页每个数据, 
    请求 https://jiugui.yyby521.xyz/user/content/view/360,
    注意, 这个 ID 顺序需要保留, 360, 但是这个不作为键值, 我们的键值依旧是 uuid, 
    
    打开这个页面存储,
    时间
    <div class="user-info-text">
        创建时间：2026-07-05 15:42:57
        
        <br>更新时间：2026-07-05 20:38:56
        
        
        <br>下次发送：2026-07-09 15:43:25
        
        <br>发送频道：未指定
    </div>

    文本内容
    <div style="color: #333; font-size: 15px; line-height: 1.8; white-space: pre-wrap; word-wrap: break-word;">编号:H0146
省份:福建
城市:泉州
年龄:08
身高:162
体重:100
罩杯:B
职ye:无
是否&nbsp;chu：否
能否👄:没有试过
能否&nbsp;SM:否
能否无🍑:(体检/试纸后):否
能否内🐍之:否
能否过夜:可以
能否同居:否
可否飞外省:本省
可否出国：否
大姨妈日期:15-22
每月陪伴天数(月/多少天):5-10
期望零花钱:2天2500
零花钱分几次：2
找金主的原因:缺钱
最早出发时间:提前沟通
自我介绍加分项：三点粉，只做过一次，会迎合老板
对老板雷点(不接受）：猎奇的东西，🚫暴力强迫
介绍费：7888
介绍人：酒鬼</div>

标题 
<h1>酒鬼bao养355</h1>

展示资料
<div style="background: #eef2f8; padding: 20px; border-radius: 12px; margin-bottom: 20px; border-left: 4px solid #4caf50;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 15px;">
            <h3 style="color: #2e7d32; margin: 0;">📤 第一次发送</h3>
            <span style="background: #4caf50; color: white; padding: 5px 15px; border-radius: 20px; font-size: 14px; font-weight: bold;">
                6 个文件
            </span>
        </div>
        
        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 15px;">
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #4caf50; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    
                        <span style="background: #ff9800; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">📷 图片</span>
                    
                    <span style="background: #e8f5e9; color: #2e7d32; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第1次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px; min-height: 180px; display: flex; align-items: center; justify-content: center;">
                    
                        <img src="/uploads/20260705154257682737_3a7dcc1e_photo_A3kAAzwE.jpg" style="max-width: 100%; max-height: 160px; border-radius: 6px; object-fit: contain; cursor: pointer;" alt="photo_A3kAAzwE.jpg" onclick="window.open(this.src, '_blank')" title="点击查看大图">
                    
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">photo_A3kAAzwE.jpg</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.06 MB</span>
                        <span>07-05 15:42</span>
                    </div>
                </div>
            </div>
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #4caf50; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    
                        <span style="background: #ff9800; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">📷 图片</span>
                    
                    <span style="background: #e8f5e9; color: #2e7d32; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第1次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px; min-height: 180px; display: flex; align-items: center; justify-content: center;">
                    
                        <img src="/uploads/20260705154258873904_a97eb6c6_photo_DeQADPAQ.jpg" style="max-width: 100%; max-height: 160px; border-radius: 6px; object-fit: contain; cursor: pointer;" alt="photo_DeQADPAQ.jpg" onclick="window.open(this.src, '_blank')" title="点击查看大图">
                    
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">photo_DeQADPAQ.jpg</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.11 MB</span>
                        <span>07-05 15:42</span>
                    </div>
                </div>
            </div>
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #4caf50; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    
                        <span style="background: #ff9800; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">📷 图片</span>
                    
                    <span style="background: #e8f5e9; color: #2e7d32; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第1次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px; min-height: 180px; display: flex; align-items: center; justify-content: center;">
                    
                        <img src="/uploads/20260705154300071395_45749898_photo_A3kAAzwE.jpg" style="max-width: 100%; max-height: 160px; border-radius: 6px; object-fit: contain; cursor: pointer;" alt="photo_A3kAAzwE.jpg" onclick="window.open(this.src, '_blank')" title="点击查看大图">
                    
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">photo_A3kAAzwE.jpg</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.12 MB</span>
                        <span>07-05 15:43</span>
                    </div>
                </div>
            </div>
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #4caf50; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    
                        <span style="background: #ff9800; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">📷 图片</span>
                    
                    <span style="background: #e8f5e9; color: #2e7d32; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第1次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px; min-height: 180px; display: flex; align-items: center; justify-content: center;">
                    
                        <img src="/uploads/20260705154301235929_ff9cd3a0_photo_DeQADPAQ.jpg" style="max-width: 100%; max-height: 160px; border-radius: 6px; object-fit: contain; cursor: pointer;" alt="photo_DeQADPAQ.jpg" onclick="window.open(this.src, '_blank')" title="点击查看大图">
                    
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">photo_DeQADPAQ.jpg</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.08 MB</span>
                        <span>07-05 15:43</span>
                    </div>
                </div>
            </div>
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #4caf50; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    
                        <span style="background: #ff9800; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">📷 图片</span>
                    
                    <span style="background: #e8f5e9; color: #2e7d32; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第1次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px; min-height: 180px; display: flex; align-items: center; justify-content: center;">
                    
                        <img src="/uploads/20260705154302397687_7ec27c8e_photo_DeQADPAQ.jpg" style="max-width: 100%; max-height: 160px; border-radius: 6px; object-fit: contain; cursor: pointer;" alt="photo_DeQADPAQ.jpg" onclick="window.open(this.src, '_blank')" title="点击查看大图">
                    
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">photo_DeQADPAQ.jpg</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.1 MB</span>
                        <span>07-05 15:43</span>
                    </div>
                </div>
            </div>
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #4caf50; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    
                        <span style="background: #ff9800; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">📷 图片</span>
                    
                    <span style="background: #e8f5e9; color: #2e7d32; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第1次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px; min-height: 180px; display: flex; align-items: center; justify-content: center;">
                    
                        <img src="/uploads/20260705154303547744_206a572b_photo_DeQADPAQ.jpg" style="max-width: 100%; max-height: 160px; border-radius: 6px; object-fit: contain; cursor: pointer;" alt="photo_DeQADPAQ.jpg" onclick="window.open(this.src, '_blank')" title="点击查看大图">
                    
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">photo_DeQADPAQ.jpg</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.1 MB</span>
                        <span>07-05 15:43</span>
                    </div>
                </div>
            </div>
            
        </div>
    </div>

    验证资源

    <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(320px, 1fr)); gap: 15px;">
            
            <div style="background: white; padding: 15px; border-radius: 10px; border: 2px solid #2196f3; transition: all 0.3s;" class="file-card">
                <!-- 文件类型标签 -->
                <div style="margin-bottom: 10px; display: flex; justify-content: space-between; align-items: center;">
                    <span style="background: #1976d2; color: white; padding: 4px 12px; border-radius: 15px; font-size: 12px; font-weight: bold;">🎬 视频</span>
                    <span style="background: #e3f2fd; color: #1565c0; padding: 3px 10px; border-radius: 12px; font-size: 11px; font-weight: bold;">第2次</span>
                </div>
                
                <!-- 文件预览 -->
                <div style="text-align: center; margin-bottom: 10px; background: #fafafa; border-radius: 8px; padding: 10px;">
                    <video style="width: 100%; max-height: 200px; border-radius: 6px;" controls="">
                        <source src="/uploads/20260705154305060539_376bed9a_20260521171111878316_3db1cdc2_fe3e7e4cebd932572af701775897524e.mp4" type="video/mp4">
                        您的浏览器不支持视频播放
                    </video>
                </div>
                
                <!-- 文件信息 -->
                <div style="background: #fafafa; padding: 10px; border-radius: 6px; font-size: 12px; color: #666;">
                    <div style="margin-bottom: 5px; word-break: break-all; color: #333; font-weight: 500;">20260521171111878316_3db1cdc2_fe3e7e4cebd932572af701775897524e.mp4</div>
                    <div style="display: flex; justify-content: space-between;">
                        <span>0.84 MB</span>
                        <span>07-05 15:43</span>
                    </div>
                </div>
            </div>
            
        </div>

        需要采集每个资料的 标题, 创建时间, 更新时间, 文本内容, 第一次发送, 第二次发送, 这些数据进行入库, 

        然后在选择在选择匹配频道, 比如说当前租户下的那些上架频道, 直接可以采集对应频道的多少天的数据, 比如说 1 天, 一周, 半年,之类的, 可以使用时间选择器, 预设几个值, 先采集资料数据, 采集完成后, 在采集 TG 频道数据, 然后和 TG 频道的 group 数据做匹配, 对应每个资料所对应的 group 消息, 使用文本匹配就行, 这样就完成了数据导入, 并且当前账号会显示所有的资料,  这里还有 COS 的迁移, COS 的迁移也做到每个资料, 使用并发下载, https://jiugui.yyby521.xyz/uploads/20260705154303547744_206a572b_photo_DeQADPAQ.jpg
         举例图片地址, 

    