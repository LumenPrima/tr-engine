// Stats management and updates
export class StatsManager {
    constructor() {
        this.stats = {
            systems: new Set(),
            units: new Set(),
            talkgroups: new Set(),
            calls: new Set()
        };
    }

    updateActivity(type) {
        const indicator = document.getElementById(`${type}-activity`);
        indicator.classList.add('active');
        setTimeout(() => {
            indicator.classList.remove('active');
        }, 1000);
    }

    flashCard(type) {
        const card = document.getElementById(`${type}-card`);
        card.classList.add('event-flash');
        setTimeout(() => {
            card.classList.remove('event-flash');
        }, 500);
    }

    updateSystemDetails(systems) {
        const tbody = document.querySelector('#systems-details .details-table tbody');
        tbody.innerHTML = '';
        systems.forEach(system => {
            const row = document.createElement('tr');
            row.innerHTML = `
                <td>${system.name}</td>
                <td>${(system.current_decoderate).toFixed(1)}/s @ ${(system.current_control_channel/1000000).toFixed(3)} MHz</td>
            `;
            tbody.appendChild(row);
        });
    }

    updateUnitDetails(units) {
        const tbody = document.querySelector('#units-details .details-table tbody');
        tbody.innerHTML = '';
        const activeUnits = units.filter(u => u.status.online).slice(0, 20);
        activeUnits.forEach(unit => {
            if (unit.status.current_talkgroup) {
                const row = document.createElement('tr');
                row.innerHTML = `
                    <td>${unit.unit}</td>
                    <td>${unit.status.current_talkgroup} (${unit.status.current_talkgroup_tag || 'Unknown'})</td>
                `;
                tbody.appendChild(row);
            }
        });
    }

    updateTalkgroupDetails(calls) {
        const tbody = document.querySelector('#talkgroups-details .details-table tbody');
        tbody.innerHTML = '';
        const activeTalkgroups = new Map();
        
        calls.forEach(call => {
            if (!activeTalkgroups.has(call.talkgroup)) {
                activeTalkgroups.set(call.talkgroup, {
                    description: call.talkgroup_description,
                    tag: call.talkgroup_tag,
                    group: call.talkgroup_group,
                    count: 1
                });
            } else {
                activeTalkgroups.get(call.talkgroup).count++;
            }
        });

        Array.from(activeTalkgroups.entries())
            .sort((a, b) => b[1].count - a[1].count)
            .slice(0, 15)
            .forEach(([tg, info]) => {
                const row = document.createElement('tr');
                row.innerHTML = `
                    <td>${tg}</td>
                    <td>${info.description} (${info.count} active)</td>
                `;
                tbody.appendChild(row);
            });
    }

    updateCallDetails(calls) {
        const tbody = document.querySelector('#calls-details .details-table tbody');
        tbody.innerHTML = '';
        calls.slice(0, 15).forEach(call => {
            const row = document.createElement('tr');
            const emergency = call.emergency ? ' <span class="emergency">[E]</span>' : '';
            row.innerHTML = `
                <td>${call.talkgroup}</td>
                <td>${call.talkgroup_description}${emergency} (${call.elapsed}s)</td>
            `;
            tbody.appendChild(row);
        });
    }

    updateCount(type, count) {
        document.getElementById(`${type}-count`).textContent = count;
    }
}

export default new StatsManager();
