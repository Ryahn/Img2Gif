import './style.css';

function escapeHtml(value) {
    return String(value)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

function normalizePadColorInput(raw) {
    const hex = String(raw).replace('#', '').slice(0, 6);
    return /^[0-9A-Fa-f]{6}$/.test(hex) ? hex.toLowerCase() : null;
}

// ─── State ───
const state = {
    folderPath: '',
    images: [],
    outputPath: '',
    version: '',
    generating: false,
    progressInterval: null,
    config: {
        width: 480,
        height: 480,
        delay: 100,
        loopCount: 0,
        fadeIn: false,
        fadeOut: false,
        fadeDuration: 0.5,
        scaleMode: 'fit',
        quality: 256,
        padColor: '000000',
    }
};

// ─── Render App ───
function renderApp() {
    const app = document.getElementById('app');
    app.innerHTML = `
        <!-- Title Bar -->
        <div class="titlebar">
            <div class="titlebar-brand">
                <div class="titlebar-icon">🎞</div>
                <span class="titlebar-title">Img2Gif</span>
                ${state.version ? `<span class="titlebar-version">v${escapeHtml(state.version)}</span>` : ''}
            </div>
            <div class="titlebar-controls">
                <button class="titlebar-btn" id="btn-minimize" title="Minimize">─</button>
                <button class="titlebar-btn" id="btn-maximize" title="Maximize">□</button>
                <button class="titlebar-btn close" id="btn-close" title="Close">✕</button>
            </div>
        </div>

        <!-- Main Content -->
        <div class="main-content">
            <!-- Left Panel: Images -->
            <div class="panel-left">
                <div class="dropzone${state.folderPath ? ' active' : ''}" id="dropzone">
                    <div class="dropzone-icon">📂</div>
                    <div class="dropzone-title">${state.folderPath ? 'Folder Selected' : 'Select Image Folder'}</div>
                    <div class="dropzone-subtitle">${state.folderPath ? 'Click to change folder' : 'Click to browse for a folder with images'}</div>
                    ${state.folderPath ? `<div class="dropzone-path">${escapeHtml(state.folderPath)}</div>` : ''}
                </div>

                ${state.images.length > 0 ? `
                    <div class="images-header">
                        <div class="images-count"><span>${state.images.length}</span> images loaded</div>
                    </div>
                    <div class="images-grid" id="images-grid">
                        ${state.images.map((img, i) => `
                            <div class="image-card" title="${escapeHtml(img.name)} (${img.width}×${img.height})">
                                <div class="image-card-index">${i + 1}</div>
                                ${img.thumbnail ? `<img src="${img.thumbnail}" alt="${escapeHtml(img.name)}" loading="lazy"/>` : `<div style="display:flex;align-items:center;justify-content:center;height:100%;color:var(--text-muted);font-size:10px;">${escapeHtml(img.name)}</div>`}
                                <div class="image-card-name">${escapeHtml(img.name)}</div>
                            </div>
                        `).join('')}
                    </div>
                ` : `
                    <div class="empty-state">
                        <div class="empty-state-icon">🖼️</div>
                        <div class="empty-state-text">No images loaded yet</div>
                    </div>
                `}
            </div>

            <!-- Right Panel: Settings -->
            <div class="panel-right">
                <div class="settings-header">
                    <span class="settings-header-icon">⚙️</span>
                    <span class="settings-header-text">Settings</span>
                </div>

                <div class="settings-body">
                    <!-- Output Size -->
                    <div class="settings-section">
                        <div class="settings-section-title">Output Size</div>
                        <div class="form-group">
                            <div class="dimension-inputs">
                                <input type="number" class="form-input" id="input-width" value="${state.config.width}" min="16" max="4096" placeholder="Width"/>
                                <span class="dimension-separator">×</span>
                                <input type="number" class="form-input" id="input-height" value="${state.config.height}" min="16" max="4096" placeholder="Height"/>
                            </div>
                        </div>
                    </div>

                    <div class="settings-divider"></div>

                    <!-- Timing -->
                    <div class="settings-section">
                        <div class="settings-section-title">Timing</div>
                        <div class="form-group">
                            <div class="form-label">
                                <span class="form-label-text">Frame Delay</span>
                                <span class="form-label-value" id="delay-value">${state.config.delay}ms</span>
                            </div>
                            <input type="range" class="form-range" id="input-delay" min="20" max="2000" step="10" value="${state.config.delay}"/>
                        </div>
                        <div class="form-group">
                            <div class="form-label">
                                <span class="form-label-text">Loop Count</span>
                                <span class="form-label-value" id="loop-value">${state.config.loopCount === 0 ? '∞ Infinite' : state.config.loopCount === -1 ? 'No Loop' : state.config.loopCount + '×'}</span>
                            </div>
                            <input type="range" class="form-range" id="input-loop" min="-1" max="20" step="1" value="${state.config.loopCount}"/>
                        </div>
                    </div>

                    <div class="settings-divider"></div>

                    <!-- Scale Mode -->
                    <div class="settings-section">
                        <div class="settings-section-title">Scale Mode</div>
                        <div class="form-group">
                            <div class="radio-group">
                                <div class="radio-option${state.config.scaleMode === 'fit' ? ' selected' : ''}" data-mode="fit">
                                    <div class="radio-dot"></div>
                                    <div>
                                        <div class="radio-label">Fit (Letterbox)</div>
                                        <div class="radio-desc">Preserves aspect ratio with bars</div>
                                    </div>
                                </div>
                                <div class="radio-option${state.config.scaleMode === 'zoom' ? ' selected' : ''}" data-mode="zoom">
                                    <div class="radio-dot"></div>
                                    <div>
                                        <div class="radio-label">Zoom (Crop)</div>
                                        <div class="radio-desc">Fills frame, crops edges</div>
                                    </div>
                                </div>
                                <div class="radio-option${state.config.scaleMode === 'stretch' ? ' selected' : ''}" data-mode="stretch">
                                    <div class="radio-dot"></div>
                                    <div>
                                        <div class="radio-label">Stretch</div>
                                        <div class="radio-desc">Forces exact dimensions</div>
                                    </div>
                                </div>
                            </div>
                        </div>
                        <div class="pad-color-group${state.config.scaleMode === 'fit' ? ' visible' : ''}" id="pad-color-section">
                            <div class="form-group">
                                <div class="form-label">
                                    <span class="form-label-text">Bar Color</span>
                                </div>
                                <div class="color-picker-row">
                                    <div class="color-preview" id="color-preview" style="background:#${state.config.padColor}"></div>
                                    <input type="text" class="form-input" id="input-padcolor" value="${state.config.padColor}" placeholder="000000" maxlength="6"/>
                                    <input type="color" id="native-color-picker" value="#${state.config.padColor}"/>
                                </div>
                            </div>
                        </div>
                    </div>

                    <div class="settings-divider"></div>

                    <!-- Crossfade -->
                    <div class="settings-section">
                        <div class="settings-section-title">Crossfade</div>
                        <div class="form-hint">Either option enables smooth transitions between frames.</div>
                        <div class="form-group">
                            <div class="checkbox-group">
                                <div class="checkbox-option${state.config.fadeIn ? ' checked' : ''}" id="cb-fadein">
                                    <div class="checkbox-box">
                                        <svg viewBox="0 0 12 12"><polyline points="2,6 5,9 10,3"></polyline></svg>
                                    </div>
                                    <span class="checkbox-label">Crossfade (start)</span>
                                </div>
                                <div class="checkbox-option${state.config.fadeOut ? ' checked' : ''}" id="cb-fadeout">
                                    <div class="checkbox-box">
                                        <svg viewBox="0 0 12 12"><polyline points="2,6 5,9 10,3"></polyline></svg>
                                    </div>
                                    <span class="checkbox-label">Crossfade (end)</span>
                                </div>
                            </div>
                        </div>
                        <div class="fade-options${(state.config.fadeIn || state.config.fadeOut) ? ' visible' : ''}" id="fade-options">
                            <div class="form-group">
                                <div class="form-label">
                                    <span class="form-label-text">Duration</span>
                                    <span class="form-label-value" id="fade-value">${state.config.fadeDuration.toFixed(1)}s</span>
                                </div>
                                <input type="range" class="form-range" id="input-fade" min="0.1" max="3" step="0.1" value="${state.config.fadeDuration}"/>
                            </div>
                        </div>
                    </div>

                    <div class="settings-divider"></div>

                    <!-- Quality -->
                    <div class="settings-section">
                        <div class="settings-section-title">Quality</div>
                        <div class="form-group">
                            <div class="form-label">
                                <span class="form-label-text">Max Colors</span>
                                <span class="form-label-value" id="quality-value">${state.config.quality}</span>
                            </div>
                            <input type="range" class="form-range" id="input-quality" min="16" max="256" step="8" value="${state.config.quality}"/>
                        </div>
                    </div>

                    <div class="settings-divider"></div>

                    <!-- Output -->
                    <div class="settings-section">
                        <div class="settings-section-title">Output</div>
                        <div class="form-group">
                            <div class="form-label">
                                <span class="form-label-text">Save Location</span>
                            </div>
                            <div class="output-row">
                                <input type="text" class="form-input" id="input-output" value="${escapeHtml(state.outputPath)}" placeholder="Auto (same folder)" readonly/>
                                <button class="btn-browse" id="btn-browse-output">Browse</button>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Generate Button -->
                <div class="generate-section">
                    <div class="generate-actions">
                        <button class="generate-btn${state.generating ? ' generating' : ''}" id="btn-generate" ${state.images.length === 0 || state.generating ? 'disabled' : ''}>
                            ${state.generating ? '<span class="spinner"></span>Generating...' : '🎬 Generate GIF'}
                        </button>
                        ${state.generating ? '<button class="cancel-btn" id="btn-cancel">Cancel</button>' : ''}
                    </div>
                    <div class="progress-container${state.generating ? ' visible' : ''}" id="progress-container">
                        <div class="progress-bar-track">
                            <div class="progress-bar-fill" id="progress-fill" style="width: 0%"></div>
                        </div>
                        <div class="progress-info">
                            <span class="progress-text" id="progress-text">Preparing...</span>
                            <span class="progress-percent" id="progress-percent">0%</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>

        <!-- Toast Container -->
        <div class="toast-container" id="toast-container"></div>
    `;

    bindEvents();
}

// ─── Event Binding ───
function bindEvents() {
    document.getElementById('btn-minimize')?.addEventListener('click', () => {
        window.runtime?.WindowMinimise();
    });
    document.getElementById('btn-maximize')?.addEventListener('click', () => {
        window.runtime?.WindowToggleMaximise();
    });
    document.getElementById('btn-close')?.addEventListener('click', () => {
        window.runtime?.Quit();
    });

    document.getElementById('dropzone')?.addEventListener('click', selectFolder);

    document.getElementById('input-width')?.addEventListener('change', (e) => {
        const val = parseInt(e.target.value);
        if (val >= 16 && val <= 4096) state.config.width = val;
    });
    document.getElementById('input-height')?.addEventListener('change', (e) => {
        const val = parseInt(e.target.value);
        if (val >= 16 && val <= 4096) state.config.height = val;
    });

    document.getElementById('input-delay')?.addEventListener('input', (e) => {
        state.config.delay = parseInt(e.target.value);
        document.getElementById('delay-value').textContent = `${state.config.delay}ms`;
    });

    document.getElementById('input-loop')?.addEventListener('input', (e) => {
        state.config.loopCount = parseInt(e.target.value);
        const label = state.config.loopCount === 0 ? '∞ Infinite' : state.config.loopCount === -1 ? 'No Loop' : state.config.loopCount + '×';
        document.getElementById('loop-value').textContent = label;
    });

    document.querySelectorAll('.radio-option').forEach(opt => {
        opt.addEventListener('click', () => {
            state.config.scaleMode = opt.dataset.mode;
            document.querySelectorAll('.radio-option').forEach(r => r.classList.remove('selected'));
            opt.classList.add('selected');
            const padSection = document.getElementById('pad-color-section');
            if (state.config.scaleMode === 'fit') {
                padSection.classList.add('visible');
            } else {
                padSection.classList.remove('visible');
            }
        });
    });

    const colorPreview = document.getElementById('color-preview');
    const nativePicker = document.getElementById('native-color-picker');
    const padInput = document.getElementById('input-padcolor');

    colorPreview?.addEventListener('click', () => nativePicker?.click());
    nativePicker?.addEventListener('input', (e) => {
        const hex = e.target.value.replace('#', '');
        state.config.padColor = hex;
        colorPreview.style.background = `#${hex}`;
        padInput.value = hex;
    });
    padInput?.addEventListener('change', (e) => {
        const hex = normalizePadColorInput(e.target.value);
        if (!hex) {
            showToast('error', 'Bar color must be a 6-digit hex value (e.g. 000000)');
            padInput.value = state.config.padColor;
            return;
        }
        state.config.padColor = hex;
        colorPreview.style.background = `#${hex}`;
        nativePicker.value = `#${hex}`;
        padInput.value = hex;
    });

    document.getElementById('cb-fadein')?.addEventListener('click', () => {
        state.config.fadeIn = !state.config.fadeIn;
        document.getElementById('cb-fadein').classList.toggle('checked');
        updateFadeVisibility();
    });
    document.getElementById('cb-fadeout')?.addEventListener('click', () => {
        state.config.fadeOut = !state.config.fadeOut;
        document.getElementById('cb-fadeout').classList.toggle('checked');
        updateFadeVisibility();
    });

    document.getElementById('input-fade')?.addEventListener('input', (e) => {
        state.config.fadeDuration = parseFloat(e.target.value);
        document.getElementById('fade-value').textContent = `${state.config.fadeDuration.toFixed(1)}s`;
    });

    document.getElementById('input-quality')?.addEventListener('input', (e) => {
        state.config.quality = parseInt(e.target.value);
        document.getElementById('quality-value').textContent = state.config.quality;
    });

    document.getElementById('btn-browse-output')?.addEventListener('click', browseOutput);
    document.getElementById('btn-generate')?.addEventListener('click', generateGif);
    document.getElementById('btn-cancel')?.addEventListener('click', cancelGenerate);
}

function updateFadeVisibility() {
    const fadeOpts = document.getElementById('fade-options');
    if (state.config.fadeIn || state.config.fadeOut) {
        fadeOpts.classList.add('visible');
    } else {
        fadeOpts.classList.remove('visible');
    }
}

function stopProgressPolling() {
    if (state.progressInterval) {
        clearInterval(state.progressInterval);
        state.progressInterval = null;
    }
}

// ─── Actions ───
async function selectFolder() {
    try {
        const path = await window.go.main.App.SelectFolder();
        if (!path) return;

        state.folderPath = path;
        state.images = [];
        renderApp();

        const images = await window.go.main.App.GetImages(path);
        state.images = images || [];

        if (state.images.length > 0 && state.images[0].width > 0) {
            state.config.width = state.images[0].width;
            state.config.height = state.images[0].height;
            if (state.config.width > 800) {
                const ratio = state.config.height / state.config.width;
                state.config.width = 800;
                state.config.height = Math.round(800 * ratio);
            }
        }

        renderApp();
    } catch (err) {
        if (err && err.toString() !== '') {
            showToast('error', `Failed to load folder: ${err}`);
        }
    }
}

async function browseOutput() {
    try {
        const path = await window.go.main.App.SelectOutputFile();
        if (path) {
            state.outputPath = path;
            document.getElementById('input-output').value = path;
        }
    } catch (err) {
        // User cancelled
    }
}

async function cancelGenerate() {
    if (!state.generating) return;
    try {
        await window.go.main.App.CancelGenerate();
    } catch (e) { /* ignore */ }
    stopProgressPolling();
    state.generating = false;
    renderApp();
    showToast('error', 'GIF generation cancelled');
}

async function generateGif() {
    if (state.images.length === 0 || state.generating) return;

    state.generating = true;
    renderApp();

    stopProgressPolling();
    state.progressInterval = setInterval(async () => {
        try {
            const progress = await window.go.main.App.GetProgress();
            updateProgress(progress);
        } catch (e) { /* ignore */ }
    }, 300);

    try {
        const config = {
            inputFolder: state.folderPath,
            outputPath: state.outputPath,
            width: state.config.width,
            height: state.config.height,
            delay: state.config.delay,
            loopCount: state.config.loopCount,
            fadeIn: state.config.fadeIn,
            fadeOut: state.config.fadeOut,
            fadeDuration: state.config.fadeDuration,
            scaleMode: state.config.scaleMode,
            quality: state.config.quality,
            padColor: state.config.padColor,
        };

        const outputPath = await window.go.main.App.GenerateGif(config);
        stopProgressPolling();
        updateProgress(100);

        state.generating = false;
        renderApp();

        showToast('success', 'GIF created successfully!', outputPath);
    } catch (err) {
        stopProgressPolling();
        state.generating = false;
        renderApp();
        const message = String(err);
        if (!message.toLowerCase().includes('cancel')) {
            showToast('error', `Failed to generate GIF: ${err}`);
        }
    }
}

function updateProgress(value) {
    const fill = document.getElementById('progress-fill');
    const percent = document.getElementById('progress-percent');
    const text = document.getElementById('progress-text');
    if (!fill) return;

    fill.style.width = `${value}%`;
    percent.textContent = `${Math.round(value)}%`;

    if (value < 10) text.textContent = 'Preparing files...';
    else if (value < 50) text.textContent = 'Processing frames...';
    else if (value < 100) text.textContent = 'Creating GIF...';
    else text.textContent = 'Complete!';
}

function showToast(type, message, filePath) {
    const container = document.getElementById('toast-container');
    if (!container) return;

    const toast = document.createElement('div');
    toast.className = `toast ${type}`;

    const iconSpan = document.createElement('span');
    iconSpan.className = 'toast-icon';
    iconSpan.textContent = type === 'success' ? '✅' : '❌';

    const msgSpan = document.createElement('span');
    msgSpan.className = 'toast-message';
    msgSpan.textContent = message;

    toast.appendChild(iconSpan);
    toast.appendChild(msgSpan);

    if (filePath) {
        const btn = document.createElement('button');
        btn.className = 'toast-action';
        btn.textContent = 'Show in Folder';
        btn.addEventListener('click', async () => {
            try {
                await window.go.main.App.OpenInExplorer(filePath);
            } catch (e) { /* ignore */ }
        });
        toast.appendChild(btn);
    }

    container.appendChild(toast);

    setTimeout(() => {
        toast.classList.add('hiding');
        setTimeout(() => toast.remove(), 300);
    }, filePath ? 8000 : 4000);
}

async function initApp() {
    try {
        state.version = await window.go.main.App.GetVersion();
    } catch (e) { /* bindings unavailable during dev */ }
    renderApp();
    try {
        const ok = await window.go.main.App.CheckFFmpeg();
        if (!ok) {
            showToast('error', 'FFmpeg was not found on PATH. Install FFmpeg to generate GIFs.');
        }
    } catch (e) { /* bindings unavailable during dev */ }
}

initApp();
