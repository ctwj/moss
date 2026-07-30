<template>

  <a-space class="mb-2" size="medium">
    <a-button size="small" type="primary" @click="onCreate">{{$t('create')}}</a-button>
    <Checkbox />
  </a-space>

  <a-table ref="table" row-key="id"
           :style="{height:tableHeight + 'px'}"
           :columns="columns"
           :data="data"
           :bordered="false"
           :loading="loading || batchDeleteLoading"
           filter-icon-align-left
           :scroll="{x:'100%', y:tableHeight-78}"
           :row-selection="rowSelection"
           v-model:selectedKeys="selectedKeys"
           v-model:pagination="pagination"
           :virtual-list-props="params.limit > 100 ? {height:'100%'}: undefined"
           @pageChange="pageChange"
           @pageSizeChange="pageSizeChange"
           @sorterChange="sorterChange"
           @filterChange="filterChange">

    <template #th>
      <th style="color:var(--color-text-3)"></th>
    </template>
    <template #td="{ record,rowIndex }">
      <td style="color:var(--color-text-1)" class="border-opacity-30" @dblclick.native="onEdit(record,rowIndex)" />
    </template>

    <template #pagination-left><div><a-spin v-if="loadingCount" /></div></template>
    <template #column-search="{ filterValue, setFilterValue, handleFilterConfirm, handleFilterReset}">
      <ColumnSearch :filterValue="filterValue" :setFilterValue="setFilterValue" :handleFilterConfirm="handleFilterConfirm" :handleFilterReset="handleFilterReset" />
    </template>

    <template #action="{ record,rowIndex }">
      <Action :record="record" :rowIndex="rowIndex" :onEdit="onEdit" :runDelete="runDelete" :modelName="modelName" />
    </template>

    <template #time="{ record,rowIndex,column }">
      <n-time v-if="record[column.dataIndex] > 0" :time="record[column.dataIndex]" :to="Date.now()/1000" type="relative" unix />
      <span v-else> - </span>
    </template>

    <template #expire_time="{ record,rowIndex,column }">
      <template v-if="record[column.dataIndex] > 0">
        <a-tag v-if="record[column.dataIndex] <= Date.now()/1000" color="red">{{ $t('expired') }}</a-tag>
        <a-tag v-else color="blue"><n-time :time="record[column.dataIndex]" :to="Date.now()/1000" type="relative" unix /></a-tag>
      </template>
      <span v-else> - </span>
    </template>

    <template #title="{ record,rowIndex,column }">
      <span @click="useOpenLink(getURL(record.slug))" style="min-width:50px"
            class="inline-block cursor-pointer hover:underline underline-offset-4 decoration-2 hover:text-blue-500">{{ record[column.dataIndex] }}</span>
    </template>

    <template #detect="{ record,rowIndex,column }">
      <span v-if="record[column.dataIndex]"><icon-clock-circle :size="16" :strokeWidth="5" :style="{color:detectDelay(record) ? '#FBBF24':'#3B82F6'}" /></span>
      <span v-else> - </span>
    </template>

    <template #linkStatus="{ record,rowIndex,column }">
      <a-switch type="line" :default-checked="record.status" @change="runLinkStatus(record.id, $event)" @dblclick.stop />
    </template>

    <template #articleStatus="{ record,rowIndex,column }">
      <a-tag
        :color="record.status ? 'green' : 'orange'"
        @click="confirmArticleStatus(record)"
        style="cursor: pointer"
      >
        {{ record.status ? $t('published') : $t('unpublished') }}
      </a-tag>
    </template>

    <template #articleDownloadStatus="{ record,rowIndex,column }">
      <a-tag
        :color="record.download_paused ? 'red' : 'green'"
        @click="confirmArticleDownloadStatus(record)"
        style="cursor: pointer"
      >
        {{ record.download_paused ? $t('downloadPaused') : $t('downloadable') }}
      </a-tag>
    </template>

    <template #category="{ record,rowIndex,column }">
      <span>{{ getCategoryName(record[column.dataIndex]) }}</span>
    </template>

    <template #url="{ record,rowIndex,column }">
      <span @click="useOpenLink" class="inline-block cursor-pointer hover:underline underline-offset-4 decoration-2 hover:text-blue-500">{{ record[column.dataIndex] }}</span>
    </template>

    <template #storePost="{ record,rowIndex,column }">
      <a-button type="outline" size="mini" @click="runStorePost(record.id)">{{$t('publish')}}</a-button>
    </template>

    <template #tag="{ record,rowIndex,column }">
      <a-space>
        <a-tag size="mini" v-for="item in record[column.dataIndex]">{{ item }}</a-tag>
      </a-space>
    </template>


  </a-table>


  <a-modal v-model:visible="visiblePost" :width="postWidth" unmountOnClose titleAlign="start" :escToClose="false"
           :mask-closable="false"
           :mask-style="{backdropFilter: 'blur(2px)'}"
           :body-style="{height:store.isMobile ? '100%':'calc(100% - 48px)', padding:store.isMobile ? '5px':'7px 10px'}"
           modal-class="data-table-modal"
           :modalStyle="{height:postHeight ? postHeight:undefined}"
           :footer="false"
  >
      <template #title>{{ postRecord.id > 0 ? $t('edit') : $t('create') }}</template>
      <a-form ref="formRef" :layout="formLayout" auto-label-width class="w-full h-full" :model="postRecord" :style="formStyle">
          <a-spin :loading="loadingGet" class="w-full h-full overflow-auto" :class="{'overflow-hidden':loadingGet}" :size="28">
            <component v-bind:is="postComponent"></component>
          </a-spin>
        <a-divider :margin="store.isMobile ? 5:10" />
        <a-space class="w-full flex justify-end">
          <a-button @click="postCancel">{{$t('cancel')}}</a-button>
          <a-button type="primary" @click="postSubmit" :disabled="loadingGet" :loading="loadingCreate || loadingUpdate">{{$t('confirm')}}</a-button>
        </a-space>
      </a-form>
  </a-modal>

  <!-- 暂停下载（版权下架）弹窗：录入正版页面 URL -->
  <a-modal v-model:visible="pauseDownloadVisible" :title="$t('pauseDownload')" :ok-text="$t('confirm')" :cancel-text="$t('cancel')" @ok="submitPauseDownload" @cancel="pauseDownloadVisible=false">
    <a-form layout="vertical">
      <a-form-item :label="$t('genuineUrl')">
        <a-input v-model="genuineUrlInput" :placeholder="$t('genuineUrlPlaceholder')" allow-clear />
      </a-form-item>
      <div class="text-gray-400 text-xs">{{ $t('confirmPauseDownloadMessage') }}</div>
    </a-form>
  </a-modal>

</template>

<script setup>
  import {useRequest} from "vue-request";
  import {ref, reactive, computed, provide} from 'vue'
  import {useStore} from "@/store/index.js";
  import {useStorage} from "@vueuse/core";
  import { Modal } from '@arco-design/web-vue';
  import Checkbox from "./Checkbox.vue";
  import ColumnSearch from "./ColumnSearch.vue";
  import Action from './slot/Action.vue'
  import { NTime } from 'naive-ui'
  import {useOpenLink} from '@/hooks/utils.js'
  import {t} from '@/locale'

  import {
    tableList,
    tableCount,
    tableDelete,
    tableBatchDelete,
    tableGet,
    tableCreate,
    tableUpdate, linkStatus, storePost, articleStatus, articleDownloadStatus,
  } from "@/api/index.js";
  import {
    batchDeleteOption,
    createOption,
    deleteOption,
    getOption,
    listOption,
    paginationOption, storePostOption,
    updateOption,
  } from './index.js'
  import {useAppendSiteURL} from "@/hooks/app/index.js";

  const {columns, order,modelName, postWidth, postHeight, postComponent, categoryTree} = defineProps({
    columns: Object,
    order: String,
    modelName:String,
    postWidth: {type:String, default:()=> 500},
    formLayout: {type:String, default:()=> 'horizontal'},
    formStyle: String,
    postHeight: String,
    postComponent: Object,
    categoryTree: {type: Array, default: () => []},
  })
  columns.push({ title: '', slotName:'action', align:'right', width:120 })

  const store =useStore()
  const table = ref(null)
  const selectedKeys = ref([]);
  const tableHeight = computed(()=>store.mainHeight - 38)
  const rowSelection = reactive({ type:'checkbox', showCheckedAll:true });
  const pagination = computed(()=> paginationOption(loadingCount, total, params, store))
  const formRef = ref(null)

  // list params
  const storageLimit = useStorage("data_table_limit", 20)
  const defaultParams = {page:1, limit:storageLimit.value, order:order, where: {field:'',operator:'',value:''}}
  const params = reactive(defaultParams)

  // post
  const visiblePost = ref(false)
  const postRecord = ref({})
  const rowIndex= ref(null)

  const postRecordGetSuccessCallback = ref(null)
  const createBeforeCallback = ref(null)
  const updateSuccessCallback = ref(null)

  // curd
  const { data, run, loading } = useRequest(tableList, listOption(modelName,params))
  const { data:total, run:runCount, loading:loadingCount } = useRequest(tableCount, {defaultParams:[modelName,params]})
  const { run:runDelete } = useRequest(tableDelete,deleteOption(refreshList))
  const { run:batchDelete, loading:batchDeleteLoading } = useRequest(tableBatchDelete,batchDeleteOption(selectedKeys,refresh))
  const { run:runGet, loading:loadingGet } = useRequest(tableGet, getOption(postRecord,postRecordGetSuccessCallback))
  const { run:runCreate, loading:loadingCreate } = useRequest(tableCreate, createOption(visiblePost, refresh, createBeforeCallback))
  const { run:runUpdate, loading:loadingUpdate } = useRequest(tableUpdate, updateOption(visiblePost, data, rowIndex, postRecord, updateSuccessCallback))
  const { run:runStorePost } = useRequest(storePost, storePostOption(refreshList))


  provide('record', postRecord)
  provide('selectedKeys', selectedKeys)
  provide('batchDelete', batchDelete)
  provide('modelName', modelName)
  provide('postRecordGetSuccessCallback', postRecordGetSuccessCallback)
  provide('createBeforeCallback', createBeforeCallback)
  provide('updateSuccessCallback', updateSuccessCallback)

  function onCreate(){
    postRecord.value = {}
    visiblePost.value = true
  }

  function onEdit(record, index){
    visiblePost.value = true
    postRecord.value = {}
    rowIndex.value = index
    runGet(modelName, record.id)
  }

  function postSubmit(){
    formRef.value.validate((errors)=>{
      if(errors!==undefined) return
      // 如果是文章模块，在保存前获取编辑器内容
      if(modelName === 'article' && postComponent.value && postComponent.value.getContent) {
        const content = postComponent.value.getContent()
        console.log('保存的文章内容:', content) // 调试输出
        if(content) {
          postRecord.value.content = content
        }
      }
      if(postRecord.value.id > 0) runUpdate(modelName, postRecord.value)
      else runCreate(modelName, postRecord.value)
    })
  }

  function postCancel(){
    visiblePost.value = false
  }

  function refresh(){
    refreshList()
    refreshCount()
  }
  provide('refresh', refresh)

  function refreshList(){
    run(modelName, params)
  }

  function refreshCount(){
    runCount(modelName, params.where)
  }

  function pageChange(n){
    params.page = n
    refreshList()
  }

  function pageSizeChange(n){
    params.page = 1
    params.limit = n
    storageLimit.value = n
    refreshList()
  }

  function sorterChange(_field, _order){
    params.page = 1
    if(!_order){
      params.order = order
    }else{
      params.order = _field + ' ' + _order.replace("end","")
    }
    refreshList()
  }

  function filterChange(field,[value,like]){
    if(value === undefined){
      params.where = {field:'',operator:'',value:''}
    }else{
      params.where.field = field
      params.where.value = value
      params.where.operator = like === true ? 'like':'equal'
    }
    params.page = 1
    refreshList()
    refreshCount()
  }

  function getURL(slug){
    let rule
    if(modelName === 'article') rule = store.config.router.article_rule
    else if(modelName === 'category')rule = store.config.router.category_rule
    else if(modelName === 'tag')rule = store.config.router.tag_rule
    return useAppendSiteURL(store, rule.replace('{slug}',slug))
  }

  const { run:runLinkStatus } = useRequest(linkStatus, {manual:true})
  const { run:runArticleStatus } = useRequest(articleStatus, {manual:true, onSuccess: refreshList})
  const { run:runArticleDownloadStatus } = useRequest(articleDownloadStatus, {manual:true, onSuccess: refreshList})

  // 下载暂停/恢复（版权下架）
  const pauseDownloadVisible = ref(false)
  const pauseRecord = ref(null)
  const genuineUrlInput = ref('')
  function confirmArticleDownloadStatus(record){
    if(record.download_paused){
      Modal.confirm({
        title: t('confirmResumeDownload'),
        content: t('confirmResumeDownloadMessage'),
        okText: t('confirm'),
        cancelText: t('cancel'),
        onOk: () => {
          runArticleDownloadStatus(record.id, { download_paused:false })
        }
      })
    } else {
      pauseRecord.value = record
      genuineUrlInput.value = record.genuine_url || ''
      pauseDownloadVisible.value = true
    }
  }
  function submitPauseDownload(){
    if(pauseRecord.value){
      runArticleDownloadStatus(pauseRecord.value.id, { download_paused:true, genuine_url: genuineUrlInput.value })
    }
    pauseDownloadVisible.value = false
  }

  function confirmArticleStatus(record){
    Modal.confirm({
      title: record.status ? t('confirmUnpublish') : t('confirmPublish'),
      content: record.status ? t('confirmUnpublishMessage') : t('confirmPublishMessage'),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: () => {
        runArticleStatus(record.id, !record.status)
      }
    })
  }

  function detectDelay(record){
    let now = new Date().getTime() / 1000
    return (now - record.create_time) < record.detect_delay*60
  }

  // 根据分类ID查找分类名称
  function getCategoryName(categoryId) {
    if (!categoryId || !categoryTree || categoryTree.length === 0) return '-'
    
    // 递归查找分类
    const findCategory = (list, id) => {
      for (const cat of list) {
        if (cat.id === id) return cat.name
        if (cat.children && cat.children.length > 0) {
          const found = findCategory(cat.children, id)
          if (found) return found
        }
      }
      return null
    }
    
    return findCategory(categoryTree, categoryId) || '-'
  }
</script>

<style>
@media (max-width:640px) {
  .data-table-modal .arco-modal-header{
    display: none;
  }
}

</style>