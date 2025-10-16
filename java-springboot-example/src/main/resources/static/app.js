// OOM 测试应用控制台 JavaScript 逻辑

// 全局变量
let memoryBlocks = [];
let logs = [];
let isConnected = false;

// API 基础配置 - 自动检测当前端口
const API_BASE_URL = `${window.location.protocol}//${window.location.host}/api/memory`;
const API_ENDPOINTS = {
    ALLOCATE: '/allocate',
    ALLOCATE_GCABLE: '/allocate-gcable',
    BATCH_ALLOCATE: '/batch-allocate',
    RELEASE: '/release',
    STATS: '/stats',
    STATS_COMPARE: '/stats-compare',
    CLEAR: '/clear',
    GC: '/gc',
    PARTIAL_RELEASE: '/partial-release',
    DETAILS: '/details',
    TEST_OOM: '/test-oom',
    HEALTH: '/health'
};

// 页面加载完成后初始化
document.addEventListener('DOMContentLoaded', function() {
    console.log('OOM 测试应用控制台已加载');
    checkBackendConnection();
    startMonitoring();
});

// 检查后端连接状态
async function checkBackendConnection() {
    try {
        const response = await fetch(`${API_BASE_URL}${API_ENDPOINTS.HEALTH}`);
        if (response.ok) {
            isConnected = true;
            updateConnectionStatus(true);
            loadInitialData();
        } else {
            throw new Error('Backend not responding');
        }
    } catch (error) {
        console.warn('Backend connection failed:', error);
        isConnected = false;
        updateConnectionStatus(false);
        showAlert('无法连接到后端服务，请确保 Spring Boot 应用正在运行', 'warning');
    }
}

// 更新连接状态显示
function updateConnectionStatus(connected) {
    const statusElement = document.querySelector('.navbar-text .bi-circle-fill');
    const statusText = document.querySelector('.navbar-text');
    
    if (connected) {
        statusElement.className = 'bi bi-circle-fill text-success';
        statusElement.title = '已连接';
        statusText.innerHTML = '<i class="bi bi-circle-fill text-success"></i> 已连接';
    } else {
        statusElement.className = 'bi bi-circle-fill text-danger';
        statusElement.title = '未连接';
        statusText.innerHTML = '<i class="bi bi-circle-fill text-danger"></i> 未连接';
    }
}

// 加载初始数据
async function loadInitialData() {
    if (!isConnected) return;
    
    try {
        await Promise.all([
            loadMemoryStats(),
            loadMemoryDetails(),
            getMemoryComparison()
        ]);
    } catch (error) {
        console.error('Failed to load initial data:', error);
        showAlert('加载初始数据失败', 'error');
    }
}

// 启动监控
function startMonitoring() {
    // 每5秒更新一次数据
    setInterval(async () => {
        if (isConnected) {
            await updateMonitoringData();
        }
    }, 5000);
}

// 更新监控数据
async function updateMonitoringData() {
    try {
        const stats = await loadMemoryStats();
        if (stats) {
            updateMonitoringDisplay(stats);
        }
    } catch (error) {
        console.error('Failed to update monitoring data:', error);
    }
}

// 更新监控显示
function updateMonitoringDisplay(stats) {
    // 更新内存使用量
    const memoryUsageElement = document.getElementById('memory-usage');
    const progressBar = document.querySelector('.card.border-primary .progress-bar');
    
    if (memoryUsageElement && progressBar) {
        const usedGB = (stats.usedMemoryMB / 1024).toFixed(1);
        memoryUsageElement.textContent = usedGB + ' GB';
        
        // 更新进度条
        const percentage = (stats.usedMemoryMB / stats.maxMemoryMB) * 100;
        progressBar.style.width = Math.min(percentage, 100) + '%';
        
        // 更新描述
        const description = progressBar.parentElement.nextElementSibling;
        if (description) {
            description.textContent = `已用: ${usedGB}GB / 总计: ${(stats.maxMemoryMB / 1024).toFixed(1)}GB`;
        }
    }
    
    // 更新统计信息
    updateStats();
}

// 申请内存
async function allocateMemory() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    const sizeMB = parseInt(document.getElementById('memory-size').value);
    const count = parseInt(document.getElementById('memory-count').value);
    
    if (sizeMB <= 0 || count <= 0) {
        showAlert('请输入有效的内存大小和数量', 'warning');
        return;
    }
    
    try {
        showAlert('正在申请内存...', 'info');
        
        if (count === 1) {
            // 单个申请
            const response = await callAPI(API_ENDPOINTS.ALLOCATE, 'POST', {
                sizeMB: sizeMB,
                requestId: `mem_${Date.now()}`
            });
            
            if (response.success) {
                addLog('info', `申请内存: ${sizeMB}MB (ID: ${response.data.requestId})`);
                showAlert(`成功申请 ${sizeMB}MB 内存`, 'success');
            } else {
                throw new Error(response.message || '申请失败');
            }
        } else {
            // 批量申请
            const response = await callAPI(API_ENDPOINTS.BATCH_ALLOCATE, 'POST', {
                sizeMB: sizeMB,
                count: count
            });
            
            if (response.success) {
                addLog('info', `批量申请内存: ${count} 个 ${sizeMB}MB 内存块`);
                showAlert(`成功批量申请 ${count} 个 ${sizeMB}MB 内存块`, 'success');
            } else {
                throw new Error(response.message || '批量申请失败');
            }
        }
        
        // 刷新数据
        await loadMemoryDetails();
        await loadMemoryStats();
        
    } catch (error) {
        console.error('Memory allocation failed:', error);
        showAlert(`申请内存失败: ${error.message}`, 'error');
        addLog('error', `申请内存失败: ${error.message}`);
    }
}

// 申请可回收内存
async function allocateGcableMemory() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    const sizeMB = parseInt(document.getElementById('gcable-memory-size').value);
    const count = parseInt(document.getElementById('gcable-memory-count').value);
    const releaseImmediately = document.getElementById('release-immediately').value === 'true';
    
    if (sizeMB <= 0 || count <= 0) {
        showAlert('请输入有效的内存大小和数量', 'warning');
        return;
    }
    
    if (count > 50) {
        showAlert('批量申请数量不能超过50个', 'warning');
        return;
    }
    
    try {
        addLog('info', `申请可回收内存: ${count} 个 ${sizeMB}MB 内存块，立即释放: ${releaseImmediately}`);
        showAlert('正在申请可回收内存...', 'info');
        
        if (count === 1) {
            // 单个申请
            const response = await callAPI(API_ENDPOINTS.ALLOCATE_GCABLE, 'POST', {
                sizeMB: sizeMB,
                requestId: `gcable_${Date.now()}`,
                releaseImmediately: releaseImmediately
            });
            
            if (response.success) {
                addLog('info', `申请可回收内存: ${sizeMB}MB (ID: ${response.data.requestId})`);
                showAlert(`成功申请 ${sizeMB}MB 可回收内存`, 'success');
            } else {
                throw new Error(response.message || '申请失败');
            }
        } else {
            // 批量申请
            for (let i = 0; i < count; i++) {
                const response = await callAPI(API_ENDPOINTS.ALLOCATE_GCABLE, 'POST', {
                    sizeMB: sizeMB,
                    requestId: `gcable_${Date.now()}_${i}`,
                    releaseImmediately: releaseImmediately
                });
                
                if (!response.success) {
                    throw new Error(`第 ${i + 1} 个内存块申请失败: ${response.message}`);
                }
                
                // 短暂延迟，避免过快申请
                await new Promise(resolve => setTimeout(resolve, 100));
            }
            
            addLog('info', `批量申请可回收内存: ${count} 个 ${sizeMB}MB 内存块`);
            showAlert(`成功批量申请 ${count} 个 ${sizeMB}MB 可回收内存块`, 'success');
        }
        
        // 刷新数据
        await loadMemoryStats();
        await getMemoryComparison();
        
    } catch (error) {
        console.error('Gcable memory allocation failed:', error);
        showAlert(`申请可回收内存失败: ${error.message}`, 'error');
        addLog('error', `申请可回收内存失败: ${error.message}`);
    }
}

// 获取内存对比分析
async function getMemoryComparison() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    try {
        const response = await callAPI(API_ENDPOINTS.STATS_COMPARE, 'GET');
        
        if (response.success) {
            updateMemoryComparisonDisplay(response.data);
            addLog('info', '获取内存对比分析完成');
        } else {
            throw new Error(response.message || '获取内存对比分析失败');
        }
    } catch (error) {
        console.error('Get memory comparison failed:', error);
        showAlert(`获取内存对比分析失败: ${error.message}`, 'error');
        addLog('error', `获取内存对比分析失败: ${error.message}`);
    }
}

// 批量申请内存
async function batchAllocate() {
    const sizeMB = parseInt(document.getElementById('memory-size').value);
    const count = parseInt(document.getElementById('memory-count').value);
    
    if (sizeMB <= 0 || count <= 0) {
        showAlert('请输入有效的内存大小和数量', 'warning');
        return;
    }
    
    if (count > 50) {
        showAlert('批量申请数量不能超过50个', 'warning');
        return;
    }
    
    const totalMemory = (sizeMB * count / 1024).toFixed(2);
    if (confirm(`确定要批量申请 ${count} 个 ${sizeMB}MB 内存块吗？\n总内存: ${totalMemory}GB`)) {
        await allocateMemory();
    }
}

// 强制垃圾回收
async function forceGC() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    try {
        addLog('warning', '触发强制垃圾回收');
        showAlert('正在执行垃圾回收...', 'info');
        
        const response = await callAPI(API_ENDPOINTS.GC, 'POST');
        
        if (response.success) {
            showAlert('垃圾回收完成', 'success');
            addLog('info', `垃圾回收完成: ${response.data.message || '清理完成'}`);
        } else {
            throw new Error(response.message || '垃圾回收失败');
        }
        
        // 刷新数据
        await loadMemoryDetails();
        await loadMemoryStats();
        
    } catch (error) {
        console.error('GC failed:', error);
        showAlert(`垃圾回收失败: ${error.message}`, 'error');
        addLog('error', `垃圾回收失败: ${error.message}`);
    }
}

// 清空所有内存
async function clearMemory() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    if (memoryBlocks.length === 0) {
        showAlert('当前没有内存块需要清理', 'info');
        return;
    }
    
    const totalMemory = getTotalMemory();
    if (confirm(`确定要清空所有内存吗？\n当前有 ${memoryBlocks.length} 个内存块，总计 ${totalMemory}MB`)) {
        try {
            const response = await callAPI(API_ENDPOINTS.CLEAR, 'DELETE');
            
            if (response.success) {
                addLog('warning', `清空所有内存，共清理 ${memoryBlocks.length} 个内存块`);
                showAlert(`成功清空所有内存，共清理 ${memoryBlocks.length} 个内存块`, 'success');
                
                // 清空本地数据
                memoryBlocks = [];
                updateMemoryBlocksTable();
                updateStats();
            } else {
                throw new Error(response.message || '清空内存失败');
            }
        } catch (error) {
            console.error('Clear memory failed:', error);
            showAlert(`清空内存失败: ${error.message}`, 'error');
            addLog('error', `清空内存失败: ${error.message}`);
        }
    }
}

// 释放指定内存块
async function releaseMemory(id) {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    try {
        const response = await callAPI(`${API_ENDPOINTS.RELEASE}/${id}`, 'DELETE');
        
        if (response.success) {
            const block = memoryBlocks.find(b => b.id === id);
            if (block) {
                addLog('info', `释放内存块: ${id} (${block.sizeMB}MB)`);
                showAlert(`成功释放内存块 ${id}`, 'success');
                
                // 从本地数据中移除
                memoryBlocks = memoryBlocks.filter(b => b.id !== id);
                updateMemoryBlocksTable();
                updateStats();
            }
        } else {
            throw new Error(response.message || '释放内存失败');
        }
    } catch (error) {
        console.error('Release memory failed:', error);
        showAlert(`释放内存失败: ${error.message}`, 'error');
        addLog('error', `释放内存失败: ${error.message}`);
    }
}

// 测试 OOM
async function testOOM() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    if (confirm('⚠️ 警告：此操作将触发 OutOfMemoryError 测试！\n应用可能会崩溃并生成堆转储文件。\n确定要继续吗？')) {
        try {
            addLog('error', '开始 OOM 测试...');
            showAlert('正在执行 OOM 测试，请等待...', 'warning');
            
            const response = await callAPI(API_ENDPOINTS.TEST_OOM, 'GET');
            
            if (response.success) {
                addLog('error', 'OOM 测试完成');
                showAlert('OOM 测试完成！请检查应用状态和堆转储文件', 'danger');
            } else {
                throw new Error(response.message || 'OOM 测试失败');
            }
            
            // 刷新数据
            await loadMemoryDetails();
            await loadMemoryStats();
            
        } catch (error) {
            console.error('OOM test failed:', error);
            showAlert(`OOM 测试失败: ${error.message}`, 'error');
            addLog('error', `OOM 测试失败: ${error.message}`);
        }
    }
}

// 刷新统计信息
async function refreshStats() {
    if (!isConnected) {
        showAlert('后端未连接，无法执行操作', 'warning');
        return;
    }
    
    try {
        await Promise.all([
            loadMemoryStats(),
            loadMemoryDetails()
        ]);
        
        addLog('info', '刷新内存统计信息');
        showAlert('统计信息已更新', 'success');
    } catch (error) {
        console.error('Failed to refresh stats:', error);
        showAlert('刷新统计信息失败', 'error');
    }
}

// 清空日志
function clearLogs() {
    if (confirm('确定要清空所有操作日志吗？')) {
        logs = [];
        updateLogs();
        showAlert('操作日志已清空', 'success');
    }
}

// 加载内存统计信息
async function loadMemoryStats() {
    try {
        const response = await callAPI(API_ENDPOINTS.STATS, 'GET');
        if (response.success) {
            const stats = response.data;
            
            // 更新监控显示
            updateMonitoringDisplay(stats);
            
            // 同时更新可回收内存统计
            updateGcableMemoryStats(stats);
            
            return stats;
        }
    } catch (error) {
        console.error('Failed to load memory stats:', error);
    }
    return null;
}

// 更新可回收内存统计
function updateGcableMemoryStats(stats) {
    const gcableTotalMemory = document.getElementById('gcable-total-memory');
    const gcableBlockCount = document.getElementById('gcable-block-count');
    
    if (gcableTotalMemory && stats.gcableMemoryMB !== undefined) {
        gcableTotalMemory.textContent = stats.gcableMemoryMB + ' MB';
    }
    
    if (gcableBlockCount && stats.gcableBlockCount !== undefined) {
        gcableBlockCount.textContent = stats.gcableBlockCount;
    }
    
    // 计算可回收内存占比
    const gcablePercent = document.getElementById('gcable-percent');
    if (gcablePercent && stats.totalMemoryUsedMB !== undefined && stats.gcableMemoryMB !== undefined) {
        const percent = stats.totalMemoryUsedMB > 0 ? (stats.gcableMemoryMB / stats.totalMemoryUsedMB * 100) : 0;
        gcablePercent.textContent = percent.toFixed(1) + '%';
    }
}

// 加载内存详情
async function loadMemoryDetails() {
    try {
        const response = await callAPI(API_ENDPOINTS.DETAILS, 'GET');
        if (response.success) {
            const details = response.data;
            
            // 更新内存块列表
            memoryBlocks = details.memoryBlocks.map(block => ({
                id: block.requestId,
                sizeMB: block.sizeMB,
                timestamp: new Date(block.timestamp).toLocaleString('zh-CN'),
                status: '活跃'
            }));
            
            updateMemoryBlocksTable();
            updateStats();
            return details;
        }
    } catch (error) {
        console.error('Failed to load memory details:', error);
    }
    return null;
}

// 更新内存块表格
function updateMemoryBlocksTable() {
    const tbody = document.getElementById('memory-blocks');
    if (!tbody) return;
    
    tbody.innerHTML = '';
    
    memoryBlocks.forEach(block => {
        const row = document.createElement('tr');
        row.innerHTML = `
            <td>${block.id}</td>
            <td><span class="badge bg-primary">${block.sizeMB} MB</span></td>
            <td>${block.timestamp}</td>
            <td><span class="badge bg-success">${block.status}</span></td>
            <td>
                <button class="btn btn-sm btn-outline-danger" onclick="releaseMemory('${block.id}')">
                    <i class="bi bi-x-circle"></i> 释放
                </button>
            </td>
        `;
        tbody.appendChild(row);
    });
}

// 更新统计信息
function updateStats() {
    const totalBlocksElement = document.getElementById('total-blocks');
    const totalMemoryElement = document.getElementById('total-memory');
    
    if (totalBlocksElement) {
        totalBlocksElement.textContent = memoryBlocks.length;
    }
    
    if (totalMemoryElement) {
        totalMemoryElement.textContent = getTotalMemory() + ' MB';
    }
}

// 更新内存对比显示
function updateMemoryComparisonDisplay(data) {
    // 更新可回收内存统计
    const gcableTotalMemory = document.getElementById('gcable-total-memory');
    const gcableBlockCount = document.getElementById('gcable-block-count');
    const gcablePercent = document.getElementById('gcable-percent');
    const memoryEfficiency = document.getElementById('memory-efficiency');
    
    if (gcableTotalMemory) {
        gcableTotalMemory.textContent = data.gcableMemory.usedMemoryMB + ' MB';
    }
    
    if (gcableBlockCount) {
        gcableBlockCount.textContent = data.gcableMemory.blockCount;
    }
    
    if (gcablePercent) {
        gcablePercent.textContent = data.analysis.gcableMemoryPercent.toFixed(1) + '%';
    }
    
    if (memoryEfficiency) {
        memoryEfficiency.textContent = data.analysis.memoryEfficiency;
        // 根据效率设置颜色
        const efficiencyColors = {
            '高': 'text-success',
            '中': 'text-warning',
            '低': 'text-danger'
        };
        memoryEfficiency.className = `text-warning ${efficiencyColors[data.analysis.memoryEfficiency] || 'text-secondary'}`;
    }
    
    // 更新内存对比分析
    const residentMemoryUsage = document.getElementById('resident-memory-usage');
    const residentBlockCount = document.getElementById('resident-block-count');
    const residentGcEffect = document.getElementById('resident-gc-effect');
    
    const gcableMemoryUsage = document.getElementById('gcable-memory-usage');
    const gcableBlockCountCompare = document.getElementById('gcable-block-count-compare');
    const gcableGcEffect = document.getElementById('gcable-gc-effect');
    
    if (residentMemoryUsage) {
        residentMemoryUsage.textContent = data.residentMemory.usedMemoryMB + ' MB';
    }
    
    if (residentBlockCount) {
        residentBlockCount.textContent = data.residentMemory.blockCount;
    }
    
    if (residentGcEffect) {
        const effectText = data.residentMemory.gcEffectMB === 0 ? '无效' : '部分有效';
        const effectClass = data.residentMemory.gcEffectMB === 0 ? 'bg-secondary' : 'bg-warning';
        residentGcEffect.textContent = effectText;
        residentGcEffect.className = `badge ${effectClass}`;
    }
    
    if (gcableMemoryUsage) {
        gcableMemoryUsage.textContent = data.gcableMemory.usedMemoryMB + ' MB';
    }
    
    if (gcableBlockCountCompare) {
        gcableBlockCountCompare.textContent = data.gcableMemory.blockCount;
    }
    
    if (gcableGcEffect) {
        const effectText = data.gcableMemory.gcEffectMB > 0 ? '有效' : '无效';
        const effectClass = data.gcableMemory.gcEffectMB > 0 ? 'bg-success' : 'bg-secondary';
        gcableGcEffect.textContent = effectText;
        gcableGcEffect.className = `badge ${effectClass}`;
    }
    
    // 更新内存管理建议
    const memoryRecommendation = document.getElementById('memory-recommendation');
    if (memoryRecommendation) {
        memoryRecommendation.textContent = data.analysis.recommendation;
    }
}

// 获取总内存使用量
function getTotalMemory() {
    return memoryBlocks.reduce((total, block) => total + block.sizeMB, 0);
}

// 添加日志
function addLog(level, message) {
    const timestamp = new Date().toLocaleString('zh-CN');
    logs.unshift({
        time: timestamp,
        level: level,
        message: message
    });
    
    // 限制日志数量
    if (logs.length > 100) {
        logs = logs.slice(0, 100);
    }
    
    updateLogs();
}

// 更新日志显示
function updateLogs() {
    const container = document.getElementById('log-container');
    if (!container) return;
    
    container.innerHTML = '';
    
    logs.forEach(log => {
        const logEntry = document.createElement('div');
        logEntry.className = 'log-entry';
        logEntry.innerHTML = `
            <span class="log-time">[${log.time}]</span>
            <span class="log-level ${log.level}">[${log.level.toUpperCase()}]</span>
            <span class="log-message">${log.message}</span>
        `;
        container.appendChild(logEntry);
    });
}

// 显示提示信息
function showAlert(message, type = 'info') {
    // 创建提示元素
    const alertDiv = document.createElement('div');
    alertDiv.className = `alert alert-${type} alert-dismissible fade show position-fixed`;
    alertDiv.style.cssText = 'top: 20px; right: 20px; z-index: 9999; min-width: 300px;';
    alertDiv.innerHTML = `
        ${message}
        <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
    `;
    
    // 添加到页面
    document.body.appendChild(alertDiv);
    
    // 自动隐藏
    setTimeout(() => {
        if (alertDiv.parentNode) {
            alertDiv.remove();
        }
    }, 5000);
}

// 格式化字节数
function formatBytes(bytes) {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// 真实 API 调用
async function callAPI(endpoint, method = 'GET', data = null) {
    const url = `${API_BASE_URL}${endpoint}`;
    const options = {
        method: method,
        headers: {
            'Content-Type': 'application/json',
        }
    };
    
    if (data && (method === 'POST' || method === 'PUT')) {
        options.body = JSON.stringify(data);
    }
    
    try {
        console.log(`API 调用: ${method} ${url}`, data);
        
        const response = await fetch(url, options);
        const responseData = await response.json();
        
        if (response.ok) {
            return {
                success: true,
                data: responseData
            };
        } else {
            return {
                success: false,
                message: responseData.message || `HTTP ${response.status}`,
                status: response.status
            };
        }
    } catch (error) {
        console.error('API call failed:', error);
        return {
            success: false,
            message: error.message || '网络请求失败'
        };
    }
}

// 键盘快捷键
document.addEventListener('keydown', function(e) {
    // Ctrl/Cmd + Enter: 申请内存
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
        e.preventDefault();
        allocateMemory();
    }
    
    // Ctrl/Cmd + G: 强制GC
    if ((e.ctrlKey || e.metaKey) && e.key === 'g') {
        e.preventDefault();
        forceGC();
    }
    
    // Ctrl/Cmd + C: 清空内存
    if ((e.ctrlKey || e.metaKey) && e.key === 'c') {
        e.preventDefault();
        clearMemory();
    }
});

// 导出函数供全局使用
window.allocateMemory = allocateMemory;
window.batchAllocate = batchAllocate;
window.forceGC = forceGC;
window.clearMemory = clearMemory;
window.releaseMemory = releaseMemory;
window.testOOM = testOOM;
window.refreshStats = refreshStats;
window.clearLogs = clearLogs;
