(function() {
    "use strict";

    var container = document.getElementById("notes-container");
    var pendingCountEl = document.getElementById("pending-count");
    var loadingMsg = document.getElementById("loading-msg");

    function fetchNotes() {
        fetch("/api/notes")
            .then(function(resp) {
                if (!resp.ok) throw new Error("HTTP " + resp.status);
                return resp.json();
            })
            .then(function(notes) {
                renderNotes(notes);
            })
            .catch(function(err) {
                container.innerHTML = '<p class="empty-state">Error loading notes: ' + escapeHTML(err.message) + '</p>';
            });
    }

    function escapeHTML(str) {
        var div = document.createElement("div");
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }

    function renderNotes(notes) {
        if (loadingMsg) {
            loadingMsg.remove();
        }

        if (!notes || notes.length === 0) {
            container.innerHTML = '<p class="empty-state">No notes awaiting approval. This page will close automatically.</p>';
            pendingCountEl.textContent = "0 pending";
            return;
        }

        var totalTargets = 0;
        notes.forEach(function(n) {
            totalTargets += (n.target_kbs || []).length;
        });
        pendingCountEl.textContent = notes.length + " note(s), " + totalTargets + " target(s) pending";

        container.innerHTML = "";
        notes.forEach(function(note) {
            var card = buildNoteCard(note);
            container.appendChild(card);
        });
    }

    function buildNoteCard(note) {
        var card = document.createElement("div");
        card.className = "note-card";
        card.setAttribute("data-filename", note.filename);

        // Metadata row
        var meta = document.createElement("div");
        meta.className = "meta";
        meta.innerHTML =
            '<span>Source: ' + escapeHTML(note.source_conversation || "N/A") + '</span>' +
            '<span>Extracted: ' + escapeHTML(note.extracted_at || "N/A") + '</span>' +
            '<span>Author: ' + escapeHTML(note.author || "N/A") + '</span>';
        card.appendChild(meta);

        // Title field (editable)
        var titleLabel = document.createElement("label");
        titleLabel.textContent = "Title";
        card.appendChild(titleLabel);

        var titleInput = document.createElement("input");
        titleInput.type = "text";
        titleInput.className = "edit-title";
        titleInput.value = note.title || "";
        card.appendChild(titleInput);

        // Content field (editable)
        var contentLabel = document.createElement("label");
        contentLabel.textContent = "Content";
        card.appendChild(contentLabel);

        var contentArea = document.createElement("textarea");
        contentArea.className = "edit-content";
        contentArea.value = note.content || "";
        card.appendChild(contentArea);

        // Target KB rows
        var targets = note.target_kbs || [];
        targets.forEach(function(kb) {
            var row = buildTargetRow(note.filename, kb, titleInput, contentArea);
            card.appendChild(row);
        });

        return card;
    }

    function buildTargetRow(filename, kb, titleInput, contentArea) {
        var row = document.createElement("div");
        row.className = "target-row";
        row.setAttribute("data-kb", kb);

        var nameSpan = document.createElement("span");
        nameSpan.className = "kb-name";
        nameSpan.textContent = kb;
        row.appendChild(nameSpan);

        var approveBtn = document.createElement("button");
        approveBtn.className = "btn-approve";
        approveBtn.textContent = "Approve";
        approveBtn.addEventListener("click", function() {
            performAction(filename, kb, "approve", titleInput, contentArea, row);
        });
        row.appendChild(approveBtn);

        var rejectBtn = document.createElement("button");
        rejectBtn.className = "btn-reject";
        rejectBtn.textContent = "Reject";
        rejectBtn.addEventListener("click", function() {
            performAction(filename, kb, "reject", null, null, row);
        });
        row.appendChild(rejectBtn);

        return row;
    }

    function performAction(filename, kb, action, titleInput, contentArea, row) {
        var buttons = row.querySelectorAll("button");
        buttons.forEach(function(b) { b.disabled = true; });

        var body;
        if (action === "approve") {
            body = JSON.stringify({
                target_kb: kb,
                title: titleInput.value,
                content: contentArea.value
            });
        } else {
            body = JSON.stringify({ target_kb: kb });
        }

        fetch("/api/notes/" + encodeURIComponent(filename) + "/" + action, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: body
        })
        .then(function(resp) {
            if (!resp.ok) {
                return resp.json().then(function(data) {
                    throw new Error(data.error || "HTTP " + resp.status);
                });
            }
            return resp.json();
        })
        .then(function(data) {
            // Show success
            var msg = document.createElement("span");
            msg.className = "status-msg success";
            msg.textContent = action === "approve" ? "Approved" : "Rejected";
            row.innerHTML = "";
            var nameSpan = document.createElement("span");
            nameSpan.className = "kb-name";
            nameSpan.textContent = kb;
            row.appendChild(nameSpan);
            row.appendChild(msg);

            // Refresh the full list after a short delay
            setTimeout(fetchNotes, 500);
        })
        .catch(function(err) {
            buttons.forEach(function(b) { b.disabled = false; });
            var existing = row.querySelector(".status-msg");
            if (existing) existing.remove();
            var msg = document.createElement("span");
            msg.className = "status-msg error";
            msg.textContent = "Error: " + err.message;
            row.appendChild(msg);
        });
    }

    // Initial load
    fetchNotes();
})();
