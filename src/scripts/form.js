import pb from "./pbinit.js";

// Get the form and input elements
const dataForm = document.getElementById('dataForm');
const nameInput = document.getElementById('nameInput');
const emailInput = document.getElementById('emailInput');
const fileInput = document.getElementById('fileInput');

// Listen to form submission
dataForm.addEventListener('submit', async function (e) {
    e.preventDefault();

    try {
        // Get the first available port
        const portNum = await findFirstAvailablePort(8090, 8190);

        // Upload the data
        const formData = new FormData();
        for (let file of fileInput.files) {
            formData.append('icon', file);
            formData.append("name", nameInput.value);
            formData.append("email", emailInput.value);
            formData.append("active", "true");
            formData.append("port", portNum);
        }

        const createdRecord = await pb.collection('caskers').create(formData);

        // Notify after successful submission
        alert("Form submitted successfully!");

    } catch (error) {
        // Handle any error that might occur during the form submission process
        console.error("Error during form submission:", error);
        alert("Error submitting form. Please try again.");
    }
});

    async function findFirstAvailablePort(start, end) {
        // Retrieve all the records
        const records = await pb.collection('caskers').getFullList({
            sort: '-port',
        });
    
        // Create a set of used ports
        const usedPorts = new Set(records.map(record => record.port));
    
        // Find the first available port in the specified range
        for (let port = start; port <= end; port++) {
            if (!usedPorts.has(port)) {
                return port; // This is the first available port in the range
            }
        }
    
        throw new Error('No available port found in the specified range.');
    }
    