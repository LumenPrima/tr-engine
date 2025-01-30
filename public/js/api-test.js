// Function to prettify JSON
function syntaxHighlight(json) {
    if (typeof json !== 'string') {
        json = JSON.stringify(json, null, 2);
    }
    json = json.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
    return json.replace(/("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g, function (match) {
        let cls = 'number';
        if (/^"/.test(match)) {
            if (/:$/.test(match)) {
                cls = 'key';
            } else {
                cls = 'string';
            }
        } else if (/true|false/.test(match)) {
            cls = 'boolean';
        } else if (/null/.test(match)) {
            cls = 'null';
        }
        return '<span class="' + cls + '">' + match + '</span>';
    });
}

// Function to display API response
async function displayResponse(url, method = 'GET') {
    const responsePanel = document.querySelector('.response-panel');
    responsePanel.innerHTML = '<div class="response-container"><pre><code>Loading...</code></pre></div>';
    
    try {
        const response = await fetch(url);
        const data = await response.json();
        const highlighted = syntaxHighlight(data);
        responsePanel.innerHTML = `
            <div class="response-container">
                <h3>Response from ${url}</h3>
                <pre><code>${highlighted}</code></pre>
            </div>
        `;
    } catch (error) {
        responsePanel.innerHTML = `
            <div class="response-container">
                <h3>Error</h3>
                <pre><code style="color: #dc3545;">${error.message}</code></pre>
            </div>
        `;
    }
}

// Fetch and populate real parameter values on page load
async function populateParameters() {
    try {
        // Fetch active calls to get real call_id
        const callsResponse = await fetch('/api/v1/calls');
        const callsData = await callsResponse.json();
        const callId = callsData.data?.calls[0]?.call_id;
        if (callId) {
            document.querySelectorAll('a[href*="12345_1234567890"]').forEach(link => {
                link.href = link.href.replace('12345_1234567890', callId);
                link.textContent = link.textContent.replace('12345_1234567890', callId);
            });
        }

        // Fetch systems to get real system name
        const systemsResponse = await fetch('/api/v1/systems');
        const systemsData = await systemsResponse.json();
        const sysName = systemsData.systems?.[0]?.sys_name;
        if (sysName) {
            document.querySelectorAll('a[href*="sys1"]').forEach(link => {
                link.href = link.href.replace('sys1', sysName);
                link.textContent = link.textContent.replace('sys1', sysName);
            });
        }

        // Fetch talkgroups to get real talkgroup ID
        const talkgroupsResponse = await fetch('/api/v1/talkgroups');
        const talkgroupsData = await talkgroupsResponse.json();
        const talkgroupId = talkgroupsData.data?.[0]?.talkgroup;
        if (talkgroupId) {
            document.querySelectorAll('a[href*="12345"]').forEach(link => {
                link.href = link.href.replace('12345', talkgroupId);
                link.textContent = link.textContent.replace('12345', talkgroupId);
            });
        }

        // Update timestamp parameters with current time
        const currentTimestamp = Math.floor(Date.now() / 1000);
        document.querySelectorAll('a[href*="time=1234567890"]').forEach(link => {
            link.href = link.href.replace('1234567890', currentTimestamp);
            link.textContent = link.textContent.replace('1234567890', currentTimestamp);
        });

        // Add loading indicator
        const loadingIndicator = document.createElement('div');
        loadingIndicator.style.backgroundColor = '#4CAF50';
        loadingIndicator.style.color = 'white';
        loadingIndicator.style.padding = '10px';
        loadingIndicator.style.position = 'fixed';
        loadingIndicator.style.top = '0';
        loadingIndicator.style.left = '0';
        loadingIndicator.style.right = '0';
        loadingIndicator.style.textAlign = 'center';
        loadingIndicator.textContent = 'Parameters populated with real data from API';
        document.body.insertBefore(loadingIndicator, document.body.firstChild);
        setTimeout(() => loadingIndicator.remove(), 3000);

    } catch (error) {
        console.error('Error populating parameters:', error);
        // Show error message
        const errorIndicator = document.createElement('div');
        errorIndicator.style.backgroundColor = '#f44336';
        errorIndicator.style.color = 'white';
        errorIndicator.style.padding = '10px';
        errorIndicator.style.position = 'fixed';
        errorIndicator.style.top = '0';
        errorIndicator.style.left = '0';
        errorIndicator.style.right = '0';
        errorIndicator.style.textAlign = 'center';
        errorIndicator.textContent = 'Error loading real parameter values. Using example values.';
        document.body.insertBefore(errorIndicator, document.body.firstChild);
        setTimeout(() => errorIndicator.remove(), 3000);
    }
}

// Update click handlers after populating parameters
function updateClickHandlers() {
    document.querySelectorAll('.endpoint a').forEach(link => {
        link.addEventListener('click', (e) => {
            e.preventDefault();
            const method = e.target.closest('.endpoint').querySelector('.method').textContent;
            displayResponse(link.href, method);
        });
    });
}

// Function to build URL with parameters
function buildUrl(baseUrl, params) {
    const url = new URL(baseUrl, window.location.origin);
    Object.entries(params).forEach(([key, value]) => {
        if (value) {
            url.searchParams.append(key, value);
        }
    });
    return url.toString();
}

// Function to handle try button click
function handleTryClick(event) {
    const endpoint = event.target.closest('.endpoint');
    const baseUrl = endpoint.dataset.baseUrl;
    const inputs = endpoint.querySelectorAll('.param-input input');
    const params = {};
    
    inputs.forEach(input => {
        if (input.value) {
            params[input.name] = input.value;
        }
    });

    const url = buildUrl(baseUrl, params);
    const method = endpoint.querySelector('.method').textContent;
    displayResponse(url, method);
}

// Function to create parameter input
function createParamInput(name, required = false) {
    return `
        <div class="param-input ${required ? 'required' : ''}">
            <label for="${name}">${name}</label>
            <input type="text" id="${name}" name="${name}" placeholder="${required ? 'Required' : 'Optional'}">
        </div>
    `;
}

// Function to add parameter inputs to endpoints
function addParameterInputs() {
    document.querySelectorAll('.endpoint').forEach(endpoint => {
        const url = endpoint.querySelector('a').getAttribute('href');
        endpoint.dataset.baseUrl = url;

        // Extract path parameters
        const pathParams = url.match(/\{([^}]+)\}/g)?.map(p => p.slice(1, -1)) || [];
        
        // Get query parameters from .params div
        const paramsDiv = endpoint.querySelector('.params');
        const queryParams = paramsDiv?.textContent.match(/params: ([^]*)/)?.[1]
            .split(',')
            .map(p => p.trim().split(' ')[0])
            .filter(p => p) || [];

        if (pathParams.length || queryParams.length) {
            const inputs = document.createElement('div');
            inputs.className = 'param-inputs';
            
            if (pathParams.length) {
                const pathGroup = document.createElement('div');
                pathGroup.className = 'param-group';
                pathGroup.innerHTML = pathParams.map(param => createParamInput(param, true)).join('');
                inputs.appendChild(pathGroup);
            }
            
            if (queryParams.length) {
                const queryGroup = document.createElement('div');
                queryGroup.className = 'param-group';
                queryGroup.innerHTML = queryParams.map(param => createParamInput(param)).join('');
                inputs.appendChild(queryGroup);
            }

            const tryButton = document.createElement('button');
            tryButton.className = 'try-button';
            tryButton.textContent = 'Try it';
            tryButton.addEventListener('click', handleTryClick);
            inputs.appendChild(tryButton);

            endpoint.appendChild(inputs);
        }
    });
}

// Add click handler setup to the populate function
window.addEventListener('load', async () => {
    await populateParameters();
    updateClickHandlers();
    addParameterInputs();
});
