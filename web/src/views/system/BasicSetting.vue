<template>
  <div class="settings-typography settings-page" v-loading="loading">
    <header class="settings-header"><div><h1>基础设置</h1><p>配置平台名称和品牌标识。</p></div><el-button type="primary" :loading="saving" @click="save" v-permission="['system-setting:edit']">保存设置</el-button></header>
    <section class="setting-card">
      <div class="section-title"><strong>品牌信息</strong><span>显示在登录页、侧边栏和浏览器标题中</span></div>
      <el-form ref="formRef" :model="form" :rules="rules" label-position="top">
        <div class="form-grid"><el-form-item label="系统名称" prop="system_name"><el-input v-model="form.system_name" maxlength="100" /></el-form-item><el-form-item label="系统副标题"><el-input v-model="form.system_subtitle" maxlength="200" /></el-form-item></div>
        <el-form-item label="系统 Logo">
          <div class="logo-control"><div class="logo-preview"><img v-if="form.logo_data" :src="form.logo_data" alt="Logo 预览" /><span v-else>枢</span></div><div><el-upload :auto-upload="false" :show-file-list="false" accept="image/png,image/jpeg,image/webp" :on-change="selectLogo"><el-button>选择图片</el-button></el-upload><el-button v-if="form.logo_data" link type="danger" @click="form.logo_data=''">移除</el-button><small>支持 PNG、JPEG、WebP，建议使用正方形图片，最大 1.5 MB。</small></div></div>
        </el-form-item>
      </el-form>
    </section>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { getSystemSettings, updateSystemSettings } from '../../api/systemSetting'
import { usePlatformStore } from '../../stores/platform'

const platform=usePlatformStore(), loading=ref(false), saving=ref(false), formRef=ref()
const form=reactive({system_name:'',system_subtitle:'',logo_data:'',default_theme:'system'})
const rules={system_name:[{required:true,message:'请输入系统名称',trigger:'blur'}]}
onMounted(load)
async function load(){loading.value=true;try{Object.assign(form,(await getSystemSettings()).data||{})}finally{loading.value=false}}
function selectLogo(file){if(file.size>1.5*1024*1024)return ElMessage.warning('Logo 图片不能超过 1.5 MB');const reader=new FileReader();reader.onload=()=>{form.logo_data=String(reader.result||'')};reader.readAsDataURL(file.raw)}
async function save(){if(!await formRef.value.validate().catch(()=>false))return;saving.value=true;try{const data={system_name:form.system_name.trim(),system_subtitle:form.system_subtitle.trim(),logo_data:form.logo_data,default_theme:form.default_theme};await updateSystemSettings(data);platform.setSettings(data);document.title=`${data.system_name}`;ElMessage.success('基础设置已保存')}finally{saving.value=false}}
</script>

<style scoped>
.settings-page{--ink:var(--ops-text);--muted:var(--ops-muted);display:flex;max-width:980px;margin:0 auto;flex-direction:column;gap:14px}.settings-header{display:flex;align-items:flex-start;justify-content:space-between;gap:20px}.settings-header h1{margin:0;color:var(--ink);font-size:21px}.settings-header p{margin:6px 0 0;color:var(--muted);font-size:12px}.setting-card{padding:22px;background:var(--ops-surface);border:1px solid var(--ops-border)}.section-title{display:flex;flex-direction:column;gap:4px;margin-bottom:20px}.section-title strong{color:var(--ops-text);font-size:14px}.section-title span{color:var(--ops-muted);font-size:11px}.form-grid{display:grid;grid-template-columns:1fr 1fr;gap:18px}.logo-control{display:flex;align-items:center;gap:16px}.logo-preview{display:grid;width:70px;height:70px;place-items:center;overflow:hidden;color:#fff;background:linear-gradient(135deg,var(--ops-primary),var(--ops-accent));font-size:24px;font-weight:800}.logo-preview img{width:100%;height:100%;object-fit:contain;background:var(--ops-surface)}.logo-control>div:last-child{display:flex;align-items:center;gap:8px}.logo-control small{display:block;color:var(--ops-muted);font-size:10px}@media(max-width:680px){.form-grid{grid-template-columns:1fr}.settings-header{align-items:flex-end}.logo-control{align-items:flex-start}.logo-control>div:last-child{align-items:flex-start;flex-wrap:wrap}}
</style>
