<template>

  <a-form-item label="完整 Cookie">
    <a-space direction="vertical" style="width: 100%">
      <a-textarea v-model="data.cookie" placeholder="请输入百度网盘完整的 Cookie" :auto-size="{minRows:3,maxRows:6}" />
      <a-button type="outline" size="small" @click="testCookie" :disabled="!data.cookie">
        测试 Cookie 有效性
      </a-button>
    </a-space>
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        登录百度网盘网页版（建议使用无痕模式），按 F12 打开开发者工具，刷新页面后从 Network 标签的 main 请求中复制完整的 Cookie 字符串。系统会自动解析出 BDUSS 和 STOKEN。
      </a-typography-text>
    </template>
  </a-form-item>

  <a-divider />

  <a-form-item :label="$t('saveDirectory')">
    <a-space direction="vertical" style="width: 100%">
      <a-input-group compact>
        <a-select
          v-model="data.save_dir"
          placeholder="选择目录或输入新目录名"
          style="width: calc(100% - 100px)"
          allow-clear
          show-search
          :filter-option="filterOption"
        >
          <a-option value="">（根目录）</a-option>
          <a-option v-for="dir in directoryList" :key="dir.server_filename" :value="dir.server_filename">
            {{ dir.server_filename }}
          </a-option>
        </a-select>
        <a-button type="primary" @click="fetchDirectoryList" :loading="loadingDirectoryList">
          刷新目录
        </a-button>
      </a-input-group>
      
      <a-input
        v-model="newDirectory"
        placeholder="输入新目录名"
        class="input"
      >
        <template #addonAfter>
          <a-button type="link" size="small" @click="createNewDirectory" :disabled="!newDirectory || !data.cookie">
            新建目录
          </a-button>
        </template>
      </a-input>
    </a-space>
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        转存文件的保存目录，可从下拉列表选择或输入新目录名。点击"刷新目录"获取百度网盘根目录列表。
      </a-typography-text>
    </template>
  </a-form-item>

  <a-form-item :label="$t('rateLimit')">
    <a-input-number v-model="data.rate_limit" class="input" :min="1" :max="100" />
    <span class="text-sm text-gray-400 ml-3">次/分钟</span>
    <template #extra>
      <a-typography-text type="secondary" class="text-xs">
        转存速率限制，建议设置为 30 次/分钟以避免触发频率限制
      </a-typography-text>
    </template>
  </a-form-item>

</template>

<script setup>
import {inject, ref, onMounted} from "vue";
import {Message} from "@arco-design/web-vue";
import axios from "@/api/axios";

const data = inject("options");
const directoryList = ref([]);
const newDirectory = ref("");
const loadingDirectoryList = ref(false);

// 过滤选项
const filterOption = (input, option) => {
  return option.value.toLowerCase().includes(input.toLowerCase());
};

// 测试 Cookie 有效性
const testCookie = async () => {
  if (!data.value.cookie) {
    Message.warning("请先配置 Cookie");
    return;
  }

  try {
    Message.info("正在测试 Cookie 有效性...");
    
    // 调用后端 API
    const response = await axios.post('/plugin/testCookie/BaiduCloudTransfer',
      JSON.stringify({ cookie: data.value.cookie }),
      {
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );
    
    if (response.data && response.data.data) {
      Message.success("Cookie 有效！可以正常使用");
    } else {
      Message.error("Cookie 无效：" + (response.data.message || "未知错误"));
    }
  } catch (error) {
    Message.error("测试 Cookie 失败：" + error.message);
  }
};

// 获取目录列表
const fetchDirectoryList = async () => {
  if (!data.value.cookie) {
    Message.warning("请先配置 Cookie");
    return;
  }

  loadingDirectoryList.value = true;
  try {
    Message.info("正在获取目录列表...");
    
    // 调用后端 API
    const response = await axios.post('/plugin/getDirectories/BaiduCloudTransfer', 
      JSON.stringify({ cookie: data.value.cookie }),
      {
        headers: {
          'Content-Type': 'application/json'
        }
      }
    );
    
    if (response.data && response.data.data) {
      directoryList.value = response.data.data || [];
      Message.success(`成功获取 ${directoryList.value.length} 个目录`);
    } else {
      Message.error("获取目录列表失败：" + (response.data.message || "未知错误"));
    }
  } catch (error) {
    Message.error("获取目录列表失败：" + error.message);
  } finally {
    loadingDirectoryList.value = false;
  }
};

// 创建新目录
const createNewDirectory = async () => {
  if (!newDirectory.value.trim()) {
    Message.warning("请输入目录名");
    return;
  }

  if (!data.value.cookie) {
    Message.warning("请先配置 Cookie");
    return;
  }

  try {
    // 注意：这里需要后端提供 API 支持
    Message.info(`创建目录: ${newDirectory.value}`);
    
    // 模拟 API 调用
    // 实际实现时需要调用后端 API
    // const response = await axios.post('/admin/api/plugin/baidu-cloud-transfer/create-directory', {
    //   cookie: data.value.cookie,
    //   directory: newDirectory.value
    // });
    
    // 成功后
    data.value.save_dir = newDirectory.value;
    newDirectory.value = "";
    Message.success("目录创建成功（需要后端 API 支持）");
    
    // 刷新目录列表
    // await fetchDirectoryList();
  } catch (error) {
    Message.error("创建目录失败：" + error.message);
  }
};

// 组件挂载时，如果已配置 Cookie，尝试获取目录列表
onMounted(() => {
  if (data.value.cookie) {
    // 暂时不自动加载，等待后端 API 支持
    // fetchDirectoryList();
  }
});
</script>

<style scoped>
.input{
  width: 100%;
}
</style>