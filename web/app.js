// API base URL - assumes API is served on same host
const API_BASE = window.location.origin;

// State
let map = null;
let heatLayer = null;
let latestMarker = null;
let allTrackers = [];
let currentTracker = null;
let currentDateRange = { days: 7 };
let currentPositions = [];
let heatMapSettings = {
    intensity: 0.3,
    radius: 25,
    blur: 15
};

// Initialize date inputs with defaults
function initializeDateInputs() {
    const end = new Date();
    const start = new Date();
    start.setDate(start.getDate() - 7);

    document.getElementById('end-date').valueAsDate = end;
    document.getElementById('start-date').valueAsDate = start;
}

// Get current date range
function getDateRange() {
    if (currentDateRange.days) {
        const end = new Date();
        const start = new Date();
        start.setDate(start.getDate() - currentDateRange.days);
        return { start, end };
    } else {
        return {
            start: new Date(document.getElementById('start-date').value),
            end: new Date(document.getElementById('end-date').value)
        };
    }
}

// Show/hide loading indicator
function setLoading(isLoading) {
    const loadingEl = document.getElementById('loading');
    const controls = document.querySelectorAll('select, button, input');

    if (isLoading) {
        loadingEl.classList.remove('hidden');
        controls.forEach(el => el.disabled = true);
    } else {
        loadingEl.classList.add('hidden');
        controls.forEach(el => el.disabled = false);
    }
}

// Show error message
function showError(message) {
    const errorEl = document.getElementById('error');
    errorEl.textContent = message;
    errorEl.classList.add('show');
    setTimeout(() => {
        errorEl.classList.remove('show');
    }, 5000);
}

// Update statistics panel
function updateStats(tracker, positions) {
    const range = getDateRange();

    document.getElementById('stat-tracker').textContent = tracker ? tracker.name : '-';
    document.getElementById('stat-count').textContent = positions ? positions.length : '-';

    if (range.start && range.end) {
        const startStr = range.start.toLocaleDateString();
        const endStr = range.end.toLocaleDateString();
        document.getElementById('stat-range').textContent = `${startStr} - ${endStr}`;
    }

    if (positions && positions.length > 0) {
        const latest = positions[0];
        const latestDate = new Date(latest.timestamp);
        document.getElementById('stat-latest').textContent = latestDate.toLocaleString();
        document.getElementById('stat-battery').textContent = `${latest.battery}%`;
    } else {
        document.getElementById('stat-latest').textContent = '-';
        document.getElementById('stat-battery').textContent = '-';
    }
}

// Fetch trackers from API
async function fetchTrackers() {
    try {
        const response = await fetch(`${API_BASE}/api/trackers`);
        if (!response.ok) {
            throw new Error(`Failed to fetch trackers: ${response.statusText}`);
        }
        const data = await response.json();
        return data.trackers || [];
    } catch (error) {
        console.error('Error fetching trackers:', error);
        throw error;
    }
}

// Fetch positions for a tracker
async function fetchPositions(trackerID, start, end) {
    try {
        const params = new URLSearchParams({
            start: start.toISOString(),
            end: end.toISOString()
        });

        const response = await fetch(`${API_BASE}/api/positions/${trackerID}?${params}`);
        if (!response.ok) {
            if (response.status === 404) {
                throw new Error(`Tracker ${trackerID} not found`);
            } else if (response.status === 400) {
                throw new Error('Invalid date format');
            }
            throw new Error(`Failed to fetch positions: ${response.statusText}`);
        }
        const data = await response.json();
        return data.positions || [];
    } catch (error) {
        console.error('Error fetching positions:', error);
        throw error;
    }
}

// Populate tracker selector
function populateTrackerSelect(trackers) {
    const select = document.getElementById('tracker-select');
    select.innerHTML = '';

    if (trackers.length === 0) {
        select.innerHTML = '<option value="">No trackers found</option>';
        return;
    }

    trackers.forEach(tracker => {
        const option = document.createElement('option');
        option.value = tracker.id;
        option.textContent = `${tracker.name} (${tracker.position_count} positions)`;
        select.appendChild(option);
    });

    // Select first tracker by default
    if (trackers.length > 0) {
        select.value = trackers[0].id;
    }
}

// Initialize the map
function initMap(center = [59.9139, 10.7522], zoom = 13) {
    if (map) {
        return; // Already initialized
    }

    map = L.map('map').setView(center, zoom);

    // Add OpenStreetMap tile layer
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
        attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
        maxZoom: 19
    }).addTo(map);
}

// Calculate center point from positions
function calculateCenter(positions) {
    if (positions.length === 0) {
        return null;
    }

    let sumLat = 0;
    let sumLng = 0;

    positions.forEach(pos => {
        sumLat += pos.lat;
        sumLng += pos.lng;
    });

    return [sumLat / positions.length, sumLng / positions.length];
}

// Render heat map from positions
function renderHeatMap(positions, resetView = true) {
    // Remove existing heat layer if present
    if (heatLayer) {
        map.removeLayer(heatLayer);
        heatLayer = null;
    }

    // Remove existing marker if present
    if (latestMarker) {
        map.removeLayer(latestMarker);
        latestMarker = null;
    }

    if (positions.length === 0) {
        showError('No positions found for this tracker in the selected date range');
        return;
    }

    // Convert positions to heat map format [lat, lng, intensity]
    const heatData = positions.map(pos => [pos.lat, pos.lng, heatMapSettings.intensity]);

    // Create heat layer with custom options
    heatLayer = L.heatLayer(heatData, {
        radius: heatMapSettings.radius,
        blur: heatMapSettings.blur,
        maxZoom: 17,
        max: 3.0,  // Increased from 1.0 to allow more dynamic range
        gradient: {
            0.0: 'blue',
            0.5: 'lime',
            0.7: 'yellow',
            1.0: 'red'
        }
    }).addTo(map);

    // Only reset view when loading new data, not when adjusting settings
    if (resetView) {
        const center = calculateCenter(positions);
        if (center) {
            map.setView(center, 13);
        }
    }

    // Add a marker for the most recent position
    if (positions.length > 0) {
        const latest = positions[0];
        const latestDate = new Date(latest.timestamp);

        latestMarker = L.marker([latest.lat, latest.lng])
            .addTo(map)
            .bindPopup(`
                <strong>Latest Position</strong><br>
                Time: ${latestDate.toLocaleString()}<br>
                Battery: ${latest.battery}%
            `)
            .openPopup();
    }
}

// Load and display tracker data
async function loadTrackerData() {
    if (!currentTracker) {
        return;
    }

    try {
        setLoading(true);

        const range = getDateRange();
        console.log(`Loading positions for tracker ${currentTracker.name} from ${range.start} to ${range.end}`);

        const positions = await fetchPositions(currentTracker.id, range.start, range.end);
        console.log(`Loaded ${positions.length} positions`);

        // Store positions in state
        currentPositions = positions;

        // Check if this is first load (map doesn't exist yet)
        const isFirstLoad = !map;

        // Initialize map if needed
        if (isFirstLoad) {
            initMap();
        }

        // Render heat map (reset view only on first load)
        renderHeatMap(positions, isFirstLoad);

        // Update stats
        updateStats(currentTracker, positions);

        setLoading(false);
    } catch (error) {
        console.error('Error loading tracker data:', error);
        setLoading(false);
        showError(error.message);
    }
}

// Setup event listeners
function setupEventListeners() {
    // Tracker selection
    document.getElementById('tracker-select').addEventListener('change', (e) => {
        const trackerId = parseInt(e.target.value);
        currentTracker = allTrackers.find(t => t.id === trackerId);
        loadTrackerData();
    });

    // Date range preset buttons
    document.querySelectorAll('.date-presets .btn').forEach(btn => {
        btn.addEventListener('click', () => {
            // Update active state
            document.querySelectorAll('.date-presets .btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');

            const days = btn.dataset.days;

            if (days === 'custom') {
                // Show custom date inputs
                document.getElementById('custom-dates').style.display = 'flex';
                currentDateRange = { custom: true };
            } else {
                // Hide custom date inputs
                document.getElementById('custom-dates').style.display = 'none';
                currentDateRange = { days: parseInt(days) };
                loadTrackerData();
            }
        });
    });

    // Custom date inputs
    document.getElementById('start-date').addEventListener('change', () => {
        if (currentDateRange.custom) {
            loadTrackerData();
        }
    });

    document.getElementById('end-date').addEventListener('change', () => {
        if (currentDateRange.custom) {
            loadTrackerData();
        }
    });

    // Heat map setting sliders
    document.getElementById('intensity-slider').addEventListener('input', (e) => {
        const value = parseFloat(e.target.value);
        heatMapSettings.intensity = value;
        document.getElementById('intensity-value').textContent = value.toFixed(2);

        // Re-render heat map with new settings, but don't reset view
        if (currentPositions.length > 0) {
            renderHeatMap(currentPositions, false);
        }
    });

    document.getElementById('radius-slider').addEventListener('input', (e) => {
        const value = parseInt(e.target.value);
        heatMapSettings.radius = value;
        document.getElementById('radius-value').textContent = value;

        // Re-render heat map with new settings, but don't reset view
        if (currentPositions.length > 0) {
            renderHeatMap(currentPositions, false);
        }
    });

    document.getElementById('blur-slider').addEventListener('input', (e) => {
        const value = parseInt(e.target.value);
        heatMapSettings.blur = value;
        document.getElementById('blur-value').textContent = value;

        // Re-render heat map with new settings, but don't reset view
        if (currentPositions.length > 0) {
            renderHeatMap(currentPositions, false);
        }
    });
}

// Main initialization function
async function init() {
    try {
        setLoading(true);

        // Initialize date inputs
        initializeDateInputs();

        // Setup event listeners
        setupEventListeners();

        // Fetch all trackers
        allTrackers = await fetchTrackers();

        if (!allTrackers || allTrackers.length === 0) {
            throw new Error('No trackers found');
        }

        console.log(`Found ${allTrackers.length} tracker(s)`);

        // Populate tracker selector
        populateTrackerSelect(allTrackers);

        // Set current tracker to first one
        currentTracker = allTrackers[0];

        // Load initial data
        await loadTrackerData();

    } catch (error) {
        console.error('Error initializing app:', error);
        setLoading(false);
        showError(error.message);

        // Initialize empty map anyway so user sees something
        if (!map) {
            initMap();
        }
    }
}

// Start the app when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
