export async function initHeader() {
    const currentPath = window.location.pathname;
    const header = document.createElement('header');
    header.className = 'site-header';
    
    const nav = document.createElement('nav');
    nav.className = 'nav-container';
    
    const title = document.createElement('a');
    title.href = '/';
    title.className = 'site-title';
    title.textContent = 'TR-Engine';
    
    const links = document.createElement('ul');
    links.className = 'nav-links';

    // Page definitions with titles and display order
    const pageDefinitions = {
        'monitor.html': { title: 'System Monitor', order: 1 },
        'dashboard.html': { title: 'System Dashboard', order: 2 },
        'recorders.html': { title: 'Recorder Status', order: 3 },
        'unit-tracker.html': { title: 'Unit Tracker', order: 4 },
        'api-test.html': { title: 'API Tester', order: 5 }
    };
    
    // Create navigation links
    Object.entries(pageDefinitions)
        .sort((a, b) => a[1].order - b[1].order)
        .forEach(([path, def]) => {
            const li = document.createElement('li');
            const a = document.createElement('a');
            a.href = '/' + path;
            a.className = 'nav-link' + (currentPath.endsWith(path) ? ' active' : '');
            a.textContent = def.title;
            li.appendChild(a);
            links.appendChild(li);
        });
    
    nav.appendChild(title);
    nav.appendChild(links);
    header.appendChild(nav);
    
    document.body.insertBefore(header, document.body.firstChild);
}
